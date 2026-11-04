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
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

func gatewayAuthentication(cfg config.AuthConfig) func(http.Handler) http.Handler {
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := coreauth.ParseBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				respondError(w, http.StatusUnauthorized, "unauthorized", "valid Gateway bearer required")
				return
			}
			id, err := coreauth.VerifyAccessToken(
				r.Context(),
				raw,
				coreauth.VerificationConfig{Issuer: cfg.Issuer, Audience: coreauth.AudienceIRGateway, Keys: cfg.Keys},
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
			next.ServeHTTP(w, r)
		})
	}
}
