package httptransport

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters"
	"github.com/sb0rka/ir/apps/gateway/internal/application"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

const testProjectID = "abcdef1234"

func TestSourcesRequiresProjectID(t *testing.T) {
	handler := testHandler(t, config.AuthConfig{Disabled: true}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", response.Code)
	}
}

func TestSourcesRequiresAndAcceptsBearerToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := config.AuthConfig{PublicKey: publicKey, Issuer: "auth.test", Audience: "api.test", Kid: "test-key", Typ: "access+jwt"}
	handler := testHandler(t, auth, nil)

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	unauthorized.Header.Set(projectIDHeader, testProjectID)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", unauthorizedResponse.Code)
	}

	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	authorized.Header.Set(projectIDHeader, testProjectID)
	authorized.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey))
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorized)
	if authorizedResponse.Code != http.StatusOK {
		t.Fatalf("got %d: %s", authorizedResponse.Code, authorizedResponse.Body.String())
	}
}

func TestMockHTTPFlows(t *testing.T) {
	handler := testHandler(t, config.AuthConfig{Disabled: true}, nil)

	sources := doRequest(t, handler, http.MethodGet, "/api/v1/sources", nil)
	var sourceBody struct {
		Items []any `json:"items"`
	}
	decodeResponse(t, sources, &sourceBody)
	if sources.Code != http.StatusOK || len(sourceBody.Items) != 5 {
		t.Fatalf("sources: status=%d items=%d body=%s", sources.Code, len(sourceBody.Items), sources.Body.String())
	}

	events := doRequest(t, handler, http.MethodPost, "/api/v1/events/search", map[string]any{"sources": []string{"maxpatrol-siem", "pt-nad"}, "limit": 100})
	var eventBody struct {
		Events []struct {
			Provenance struct {
				Source string `json:"source"`
			} `json:"provenance"`
		} `json:"events"`
	}
	decodeResponse(t, events, &eventBody)
	seen := map[string]bool{}
	for _, event := range eventBody.Events {
		seen[event.Provenance.Source] = true
	}
	if events.Code != http.StatusOK || !seen["maxpatrol-siem"] || !seen["pt-nad"] {
		t.Fatalf("event fan-out failed: status=%d sources=%v body=%s", events.Code, seen, events.Body.String())
	}

	fusion := doRequest(t, handler, http.MethodPost, "/api/v1/entities/lookup", map[string]any{"sources": []string{"pt-fusion"}, "entity": map[string]string{"type": "ip", "value": "10.125.11.90"}})
	if fusion.Code != http.StatusOK || !bytes.Contains(fusion.Body.Bytes(), []byte(`"malicious"`)) {
		t.Fatalf("fusion: status=%d body=%s", fusion.Code, fusion.Body.String())
	}

	sandbox := doRequest(t, handler, http.MethodPost, "/api/v1/artifact-analyses", map[string]any{"source": "pt-sandbox", "artifact": map[string]string{"name": "shell.php"}})
	if sandbox.Code != http.StatusAccepted || !bytes.Contains(sandbox.Body.Bytes(), []byte(`"completed"`)) {
		t.Fatalf("sandbox: status=%d body=%s", sandbox.Code, sandbox.Body.String())
	}

	endpoints := doRequest(t, handler, http.MethodPost, "/api/v1/endpoints/search", map[string]any{"sources": []string{"maxpatrol-edr"}})
	var endpointBody struct {
		Items []struct {
			ExternalID string `json:"external_id"`
		} `json:"items"`
	}
	decodeResponse(t, endpoints, &endpointBody)
	if endpoints.Code != http.StatusOK || len(endpointBody.Items) == 0 {
		t.Fatalf("endpoints: status=%d body=%s", endpoints.Code, endpoints.Body.String())
	}
	actions := doRequest(t, handler, http.MethodGet, "/api/v1/sources/maxpatrol-edr/endpoints/"+endpointBody.Items[0].ExternalID+"/response-actions", nil)
	if actions.Code != http.StatusOK || !bytes.Contains(actions.Body.Bytes(), []byte(`"isolate_network"`)) {
		t.Fatalf("actions: status=%d body=%s", actions.Code, actions.Body.String())
	}
}

func TestValidationAndCORS(t *testing.T) {
	handler := testHandler(t, config.AuthConfig{Disabled: true}, map[string]bool{"http://console.test": true})

	invalid := doRequest(t, handler, http.MethodPost, "/api/v1/events/search", map[string]any{"limit": 101})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", invalid.Code)
	}
	unknown := doRequest(t, handler, http.MethodPost, "/api/v1/events/search", map[string]any{"vendor_url": "https://example.test"})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field got %d, want 400", unknown.Code)
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/events/search", nil)
	preflight.Header.Set("Origin", "http://console.test")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Headers") != "Content-Type, Authorization, X-Project-ID" {
		t.Fatalf("unexpected preflight: status=%d headers=%v", preflightResponse.Code, preflightResponse.Header())
	}
}

func TestProjectSourceAllowlistFiltersAndRejects(t *testing.T) {
	handler := testHandlerWithConfig(t, config.Config{
		Auth: config.AuthConfig{Disabled: true},
		ProjectSources: map[string]map[string]bool{
			testProjectID: {"pt-nad": true},
		},
	})
	sources := doRequest(t, handler, http.MethodGet, "/api/v1/sources", nil)
	var sourceBody struct {
		Items []struct {
			Code string `json:"code"`
		} `json:"items"`
	}
	decodeResponse(t, sources, &sourceBody)
	if len(sourceBody.Items) != 1 || sourceBody.Items[0].Code != "pt-nad" {
		t.Fatalf("unexpected sources: %s", sources.Body.String())
	}

	denied := doRequest(t, handler, http.MethodPost, "/api/v1/events/search", map[string]any{"sources": []string{"maxpatrol-siem"}})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403: %s", denied.Code, denied.Body.String())
	}
	allowed := doRequest(t, handler, http.MethodPost, "/api/v1/events/search", map[string]any{})
	if allowed.Code != http.StatusOK || bytes.Contains(allowed.Body.Bytes(), []byte(`"maxpatrol-siem"`)) {
		t.Fatalf("allowlist was not applied to fan-out: %s", allowed.Body.String())
	}
}

func testHandler(t *testing.T, auth config.AuthConfig, whitelist map[string]bool) http.Handler {
	t.Helper()
	return testHandlerWithConfig(t, config.Config{Auth: auth, Server: config.ServerConfig{CORSWhitelist: whitelist}})
}

func testHandlerWithConfig(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := adapters.NewMockRegistry(value)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(cfg, log, application.New(providers, time.Second, time.Second))
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set(projectIDHeader, testProjectID)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v: %s", err, response.Body.String())
	}
}

func signedToken(t *testing.T, privateKey ed25519.PrivateKey) string {
	t.Helper()
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub": "user-1", "sid": "session-1", "sk": "user", "jti": "token-1",
		"iss": "auth.test", "aud": []string{"api.test"}, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = "test-key"
	token.Header["typ"] = "access+jwt"
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
