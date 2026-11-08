package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/irconfig"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

func gatewayAuthentication(cfg config.AuthConfig, agents *AgentAccess) func(http.Handler) http.Handler {
	configurationClient, configurationErr := common.NewConfigurationClient(cfg.APIBaseURL)
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if len(r.Header.Values("Authorization")) != 1 {
				respondError(w, http.StatusUnauthorized, "unauthorized", "Gateway and configuration bearers required")
				return
			}
			raw, err := coreauth.ParseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				respondError(w, http.StatusUnauthorized, "unauthorized", "valid Gateway bearer required")
				return
			}
			if strings.HasPrefix(raw, "s0_ir_agent_") {
				agents.serve(next, w, r, raw)
				return
			}
			if len(r.Header.Values(irconfig.TokenHeader)) != 1 {
				respondError(w, http.StatusUnauthorized, "unauthorized", "Gateway and configuration bearers required")
				return
			}
			id, err := coreauth.VerifyAccessToken(
				r.Context(),
				raw,
				coreauth.VerificationConfig{Issuer: cfg.Issuer, Audience: coreauth.AudienceIRGateway, Keys: cfg.Keys, Policy: cfg.AccessTokenPolicy},
			)
			if errors.Is(err, coreauth.ErrKeysUnavailable) {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "verification keys unavailable")
				return
			}
			if err != nil || id.SubjectKind != coreauth.SubjectUser || id.ActorSubjectID == "" {
				respondError(w, http.StatusUnauthorized, "unauthorized", "valid delegated Gateway token required")
				return
			}
			project := strings.TrimSpace(r.Header.Get(projectIDHeader))
			if !slices.Contains(strings.Fields(id.Scope), coreauth.ScopeIRGatewayUse) ||
				!slices.Contains(id.ProjectIDs, project) {
				respondError(w, http.StatusForbidden, "forbidden", "project or Gateway scope not granted")
				return
			}
			values := url.Values{"token": {raw}, "token_type_hint": {"access_token"}}
			request, err := http.NewRequestWithContext(
				r.Context(),
				http.MethodPost,
				cfg.Issuer+"/oauth2/introspect",
				strings.NewReader(values.Encode()),
			)
			if err != nil {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "authorization unavailable")
				return
			}
			request.SetBasicAuth(url.QueryEscape(cfg.ClientID), url.QueryEscape(string(cfg.ClientSecret)))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response, err := client.Do(request)
			if err != nil {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "authorization unavailable")
				return
			}
			defer response.Body.Close()
			var active coreauth.IntrospectionResponse
			if response.StatusCode != http.StatusOK ||
				json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&active) != nil {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "authorization unavailable")
				return
			}
			if !active.Active || active.Subject != id.SubjectID || active.SessionID != id.SessionID ||
				active.ClientID != id.ClientID ||
				active.Audience != coreauth.AudienceIRGateway ||
				active.Actor != id.ActorSubjectID ||
				active.DelegationID != id.DelegationID ||
				active.DelegationVersion != id.DelegationVersion {
				respondError(w, http.StatusForbidden, "forbidden", "delegation inactive")
				return
			}
			// Introspection is checked on every request, including cached source metadata.
			// Vendor credentials are never resolved using this Gateway-audience token.
			configRaw, err := coreauth.ParseBearerToken(r.Header.Get(irconfig.TokenHeader))
			if err != nil {
				respondError(w, http.StatusUnauthorized, "unauthorized", "configuration bearer required")
				return
			}
			configID, err := coreauth.VerifyAccessToken(r.Context(), configRaw, coreauth.VerificationConfig{Issuer: cfg.Issuer, Audience: coreauth.AudienceAPI, Keys: cfg.Keys, Policy: cfg.AccessTokenPolicy})
			if errors.Is(err, coreauth.ErrKeysUnavailable) {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "verification keys unavailable")
				return
			}
			configScopes := strings.Fields(configID.Scope)
			if err != nil || configID.SubjectKind != coreauth.SubjectUser || configID.SubjectID != id.SubjectID || configID.SessionID != id.SessionID || configID.ClientID != id.ClientID || configID.ActorSubjectID == "" || configID.ActorSubjectID != id.ActorSubjectID || len(configScopes) != 2 || !slices.Contains(configScopes, coreauth.ScopeSecretsRead) || !slices.Contains(configScopes, coreauth.ScopeSecretsReveal) || !slices.Contains(configID.ProjectIDs, project) {
				respondError(w, http.StatusForbidden, "forbidden", "configuration identity or project does not match Gateway")
				return
			}
			if configurationErr != nil {
				respondError(w, http.StatusServiceUnavailable, "auth_unavailable", "configuration service unavailable")
				return
			}
			// This request checks API-grant liveness and current user RBAC even if
			// the response uses cached source metadata or there are no sources.
			requestContext := common.WithConfigurationSnapshot(r.Context())
			if _, err := configurationClient.Snapshot(requestContext, configRaw, project); err != nil {
				status := http.StatusServiceUnavailable
				if errors.Is(err, common.ErrUnauthorized) || errors.Is(err, common.ErrForbidden) || errors.Is(err, common.ErrSecretNotFound) {
					status = http.StatusForbidden
				}
				respondError(w, status, "configuration_unavailable", "configuration authorization unavailable")
				return
			}
			next.ServeHTTP(w, r.WithContext(requestContext))
		})
	}
}
