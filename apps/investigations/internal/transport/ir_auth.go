package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	authentication "github.com/sb0rka/ir/packages/contract/authentication"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

func (h *AuthHandler) refreshCookieName() string {
	if h.cfg.CookieSecure {
		return "__Host-ir_refresh"
	}
	return "ir_refresh"
}

type AuthHandler struct {
	flow *authflow.Service
	cfg  config.AuthConfig
	log  *slog.Logger
}

func NewAuthHandler(flow *authflow.Service, cfg config.AuthConfig, log *slog.Logger) *AuthHandler {
	return &AuthHandler{flow: flow, cfg: cfg, log: log}
}

func writeAuthError(w http.ResponseWriter, log *slog.Logger, err error) {
	status, code, message := http.StatusServiceUnavailable, httperr.CodeSourceUnavailable, "Authentication or authorization unavailable"
	switch {
	case errors.Is(err, authflow.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, httperr.CodeUnauthorized, "Authentication required"
	case errors.Is(err, authflow.ErrForbidden):
		status, code, message = http.StatusForbidden, httperr.CodeForbidden, "Integration consent required"
	case errors.Is(err, authflow.ErrNotFound):
		status, code, message = http.StatusNotFound, httperr.CodeNotFound, "Project not found"
	case errors.Is(err, authflow.ErrInvalid):
		status, code, message = http.StatusBadRequest, httperr.CodeValidation, "Invalid authentication request"
	case errors.Is(err, authflow.ErrConflict):
		status, code, message = http.StatusConflict, httperr.CodeValidation, "Configuration changed; reload before saving"
	}
	w.Header().Set("Cache-Control", "no-store")
	httperr.Write(w, log, httperr.New(status, code, message))
}

func authJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func authBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return authflow.ErrInvalid
	}
	return nil
}

func (h *AuthHandler) origin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != h.cfg.WebURL {
		writeAuthError(w, h.log, authflow.ErrForbidden)
		return false
	}
	return true
}

func (h *AuthHandler) authenticated(w http.ResponseWriter, r *http.Request) (model.LoginSession, string, bool) {
	raw, err := coreauth.ParseBearerToken(r.Header.Get("Authorization"))
	if err != nil {
		writeAuthError(w, h.log, authflow.ErrUnauthorized)
		return model.LoginSession{}, "", false
	}
	session, _, err := h.flow.Authenticate(r.Context(), raw)
	if err != nil {
		writeAuthError(w, h.log, err)
		return session, "", false
	}
	return session, raw, true
}

func (h *AuthHandler) cookie(w http.ResponseWriter, handle string, expires time.Time) {
	maxAge := int(time.Until(expires) / time.Second)
	if handle == "" {
		maxAge = -1
	}
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     h.refreshCookieName(),
			Value:    handle,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.cfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			Expires:  expires,
			MaxAge:   maxAge,
		},
	)
}

func (h *AuthHandler) localHandle(r *http.Request) string {
	c, err := r.Cookie(h.refreshCookieName())
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *AuthHandler) InitIRLogin(w http.ResponseWriter, r *http.Request, _ authentication.InitIRLoginParams) {
	if !h.origin(w, r) {
		return
	}
	var body authentication.InitLoginRequest
	if err := authBody(w, r, &body); err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	target, err := h.flow.InitLogin(r.Context(), body.AppChallenge)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, authentication.AuthorizeResponse{AuthorizeUrl: target})
}

func (h *AuthHandler) IrLoginCallback(
	w http.ResponseWriter,
	r *http.Request,
	params authentication.IrLoginCallbackParams,
) {
	target, err := h.flow.LoginCallback(r.Context(), params.State, r.URL.Query().Get("code"))
	if err != nil {
		target = h.cfg.WebURL + "/?auth_error=login_failed"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *AuthHandler) RedeemIRLogin(w http.ResponseWriter, r *http.Request, _ authentication.RedeemIRLoginParams) {
	if !h.origin(w, r) {
		return
	}
	var body authentication.RedeemLoginRequest
	if err := authBody(w, r, &body); err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	session, access, handle, err := h.flow.Redeem(r.Context(), body.Handoff, body.AppVerifier)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	h.cookie(w, handle, session.ExpiresAt)
	authJSON(w, authentication.AccessTokenResponse{AccessToken: access})
}

func (h *AuthHandler) RefreshIRSession(
	w http.ResponseWriter,
	r *http.Request,
	_ authentication.RefreshIRSessionParams,
) {
	if !h.origin(w, r) {
		return
	}
	session, access, handle, err := h.flow.RefreshRotating(r.Context(), h.localHandle(r))
	if err != nil {
		if errors.Is(err, authflow.ErrUnauthorized) {
			h.cookie(w, "", time.Unix(1, 0))
		}
		writeAuthError(w, h.log, err)
		return
	}
	h.cookie(w, handle, session.ExpiresAt)
	authJSON(w, authentication.AccessTokenResponse{AccessToken: access})
}

func (h *AuthHandler) LogoutIRSession(w http.ResponseWriter, r *http.Request, _ authentication.LogoutIRSessionParams) {
	if !h.origin(w, r) {
		return
	}
	if err := h.flow.Logout(r.Context(), h.localHandle(r)); err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	h.cookie(w, "", time.Unix(1, 0))
	authJSON(w, map[string]bool{"logged_out": true})
}

func (h *AuthHandler) GetIRProfile(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authenticated(w, r)
	if !ok {
		return
	}
	authJSON(w, json.RawMessage(session.Profile))
}

func (h *AuthHandler) ConnectIRIntegration(
	w http.ResponseWriter,
	r *http.Request,
	integration authentication.ConnectIRIntegrationParamsIntegration,
	_ authentication.ConnectIRIntegrationParams,
) {
	if !h.origin(w, r) {
		return
	}
	session, raw, ok := h.authenticated(w, r)
	if !ok {
		return
	}
	target, err := h.flow.Connect(r.Context(), session, raw, string(integration))
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, authentication.AuthorizeResponse{AuthorizeUrl: target})
}

func (h *AuthHandler) IrIntegrationCallback(
	w http.ResponseWriter,
	r *http.Request,
	integration authentication.IrIntegrationCallbackParamsIntegration,
	params authentication.IrIntegrationCallbackParams,
) {
	target, err := h.flow.IntegrationCallback(r.Context(), string(integration), params.State, r.URL.Query().Get("code"))
	if err != nil {
		target = h.cfg.WebURL + "/?integration_error=consent_failed"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *AuthHandler) IrIntegrationStatus(
	w http.ResponseWriter,
	r *http.Request,
	integration authentication.IrIntegrationStatusParamsIntegration,
) {
	_, raw, ok := h.authenticated(w, r)
	if !ok {
		return
	}
	grant, err := h.flow.GrantStatus(r.Context(), raw, string(integration))
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, grant)
}

func (h *AuthHandler) DisconnectIRIntegration(
	w http.ResponseWriter,
	r *http.Request,
	integration authentication.DisconnectIRIntegrationParamsIntegration,
	_ authentication.DisconnectIRIntegrationParams,
) {
	if !h.origin(w, r) {
		return
	}
	_, raw, ok := h.authenticated(w, r)
	if !ok {
		return
	}
	if err := h.flow.Disconnect(r.Context(), raw, string(integration)); err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, map[string]bool{"connected": false})
}

func (h *AuthHandler) ListIRProjects(w http.ResponseWriter, r *http.Request) {
	session, raw, ok := h.authenticated(w, r)
	if !ok {
		return
	}
	projects, err := h.flow.Projects(r.Context(), session, raw)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, projects)
}
