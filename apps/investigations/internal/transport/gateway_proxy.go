package transport

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/common/irconfig"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

// gatewayProxy permits only documented operations on the fixed Gateway URL.
func gatewayProxy(deps Dependencies) http.Handler {
	target, _ := url.Parse(deps.AuthFlow.GatewayBaseURL())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, baseURL+"/gateway")
		if !gatewayOperationAllowed(r.Method, path) {
			http.NotFound(w, r)
			return
		}
		if len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values(projectIDHeader)) > 1 {
			writeAuthError(w, deps.Log, authflow.ErrUnauthorized)
			return
		}
		raw, err := coreauth.ParseBearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeAuthError(w, deps.Log, authflow.ErrUnauthorized)
			return
		}
		project := strings.TrimSpace(r.Header.Get(projectIDHeader))
		if !validProjectID(project) {
			writeAuthError(w, deps.Log, authflow.ErrInvalid)
			return
		}
		if _, err := deps.AuthFlow.AuthenticateProjectRequest(r.Context(), raw, project); err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		token, err := deps.AuthFlow.GatewayToken(r.Context(), raw, project)
		if err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		configToken, err := deps.AuthFlow.GatewayConfigToken(r.Context(), raw, project)
		if err != nil {
			writeAuthError(w, deps.Log, err)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Director = func(request *http.Request) {
			request.URL.Scheme, request.URL.Host, request.URL.Path, request.URL.RawPath = target.Scheme, target.Host, path, ""
			request.Host = target.Host
			request.Header.Del("Cookie")
			request.Header.Del("Origin")
			request.Header.Del("Proxy-Authorization")
			request.Header.Del("X-Forwarded-Host")
			request.Header.Del("X-Forwarded-Proto")
			request.Header.Del("X-Forwarded-For")
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Del(irconfig.TokenHeader)
			if configToken != "" {
				request.Header.Set(irconfig.TokenHeader, "Bearer "+configToken)
			}
			request.Header.Set(projectIDHeader, project)
		}
		proxy.ModifyResponse = sanitizeGatewayResponse
		proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
			httperr.Write(w, deps.Log, httperr.New(http.StatusServiceUnavailable, httperr.CodeSourceUnavailable, "Gateway unavailable"))
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		w.Header().Set("Cache-Control", "no-store")
		proxy.ServeHTTP(w, r)
	})
}

// The IR boundary owns browser CORS. Combining its headers with Gateway's
// headers creates duplicate Allow-Origin values which browsers reject.
func sanitizeGatewayResponse(response *http.Response) error {
	response.Header.Del("Set-Cookie")
	for key := range response.Header {
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			response.Header.Del(key)
		}
	}
	response.Header.Set("Cache-Control", "no-store")
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return authflow.ErrUnavailable
	}
	return nil
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
	if path == "/api/v1/sources" || path == "/api/v1/events/fields" {
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
