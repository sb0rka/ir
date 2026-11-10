package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
)

func TestCORSAllowsAuthorizationForConfiguredOrigin(t *testing.T) {
	t.Parallel()

	cfg := config.ServerConfig{CORSWhitelist: map[string]bool{"http://localhost:5173": true}}
	handler := corsMiddleware(cfg)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("preflight reached next handler")
	}))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/investigations", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, X-Project-ID" {
		t.Fatalf("allow headers=%q", got)
	}
}

func TestMCPOriginMiddlewareRejectsUnknownBrowserOrigin(t *testing.T) {
	t.Parallel()
	cfg := config.ServerConfig{CORSWhitelist: map[string]bool{"http://localhost:5173": true}}
	handler := mcpOriginMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("non-browser client got %d, want 204", recorder.Code)
	}
}
