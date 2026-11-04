package transport

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

// gatewayProxy permits only the documented Gateway operations on a fixed
// deployment URL. It never forwards cookies or the caller's IR access token.
func gatewayProxy(deps Dependencies) http.Handler {
	target, _ := url.Parse(deps.AuthFlow.GatewayBaseURL())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, baseURL+"/gateway")
		if !gatewayOperationAllowed(r.Method, path) {
			http.NotFound(w, r)
			return
		}
		raw, err := coreauth.ParseBearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeAuthError(w, deps.Log, authflow.ErrUnauthorized)
			return
		}
		session, _, err := deps.AuthFlow.Authenticate(r.Context(), raw)
		if err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		project := strings.TrimSpace(r.Header.Get(projectIDHeader))
		if !validProjectID(project) {
			writeAuthError(w, deps.Log, authflow.ErrInvalid)
			return
		}
		if err := deps.AuthFlow.CheckProject(r.Context(), session, raw, project); err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		token, err := deps.AuthFlow.DelegatedToken(
			r.Context(),
			session,
			raw,
			"gateway",
			[]string{coreauth.ScopeIRGatewayUse},
			project,
		)
		if err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Director = func(request *http.Request) {
			request.URL.Scheme, request.URL.Host, request.URL.Path, request.URL.RawPath = target.Scheme, target.Host, path, ""
			request.Host = target.Host
			request.Header.Del("Cookie")
			request.Header.Del("Proxy-Authorization")
			request.Header.Del("X-Forwarded-Host")
			request.Header.Del("X-Forwarded-Proto")
			request.Header.Del("X-Forwarded-For")
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set(projectIDHeader, project)
		}
		proxy.ModifyResponse = func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Set("Cache-Control", "no-store")
			if response.StatusCode >= 300 && response.StatusCode < 400 {
				return authflow.ErrUnavailable
			}
			return nil
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeAuthError(w, deps.Log, authflow.ErrUnavailable)
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		w.Header().Set("Cache-Control", "no-store")
		proxy.ServeHTTP(w, r)
	})
}

func gatewayOperationAllowed(method, path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "v1" {
		return false
	}
	if method == http.MethodPost {
		switch path {
		case "/api/v1/findings/search",
			"/api/v1/events/search",
			"/api/v1/events/aggregate",
			"/api/v1/sessions/search",
			"/api/v1/entities/lookup",
			"/api/v1/endpoints/search",
			"/api/v1/context/resolve",
			"/api/v1/evidence/exports",
			"/api/v1/artifact-analyses":
			return true
		}
	}
	if method != http.MethodGet {
		return false
	}
	if path == "/api/v1/sources" {
		return true
	}
	if len(parts) == 4 && parts[2] == "artifact-analyses" {
		return true
	}
	if len(parts) == 5 && parts[2] == "evidence" && parts[3] == "exports" {
		return true
	}
	if len(parts) == 6 && parts[2] == "evidence" && parts[3] == "exports" && parts[5] == "content" {
		return true
	}
	if len(parts) >= 6 && parts[2] == "sources" {
		switch parts[4] {
		case "account":
			return len(parts) == 6 && parts[5] == "userinfo"
		case "sessions":
			return len(parts) == 6
		case "findings":
			return len(parts) == 7
		case "endpoints":
			return len(parts) == 7 && parts[6] == "response-actions"
		}
	}
	return false
}
