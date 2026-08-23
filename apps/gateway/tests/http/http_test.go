package http_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"

	adaptermock "github.com/sb0rka/ir/apps/gateway/internal/adapters/mock"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	httptransport "github.com/sb0rka/ir/apps/gateway/internal/transport/http"
)

const projectID = "abcdef1234"

func TestProjectAccessIsDenyByDefault(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(false)
	cfg.Auth.PublicKey = publicKey
	token := signToken(t, privateKey, cfg.Auth)

	for _, test := range []struct {
		name      string
		projectID string
		status    int
	}{
		{name: "unconfigured project", projectID: "aabbccddee", status: http.StatusForbidden},
		{name: "configured project", projectID: projectID, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler(t, cfg)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("X-Project-ID", test.projectID)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d", response.Code, test.status)
			}
		})
	}
}

func TestAuthUsesConfiguredAudience(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(false)
	cfg.Auth.PublicKey = publicKey
	cfg.Auth.Audience = "custom-audience.local"
	handler := newHandler(t, cfg)

	for _, test := range []struct {
		name     string
		audience string
		status   int
	}{
		{name: "configured audience", audience: cfg.Auth.Audience, status: http.StatusOK},
		{name: "different audience", audience: "other.local", status: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokenConfig := cfg.Auth
			tokenConfig.Audience = test.audience
			request := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
			request.Header.Set("Authorization", "Bearer "+signToken(t, privateKey, tokenConfig))
			request.Header.Set("X-Project-ID", projectID)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestEventSearchEnforcesOpenAPIInputLimits(t *testing.T) {
	handler := newHandler(t, testConfig(true))
	requests := []string{
		`{"query":"` + strings.Repeat("x", 1001) + `"}`,
		`{"entities":[` + strings.Repeat(`{"type":"ip","value":"192.0.2.1"},`, 100) + `{"type":"ip","value":"192.0.2.2"}]}`,
	}
	for _, body := range requests {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/events/search", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Project-ID", projectID)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestOmittedSourcesUseAllowedCapabilityIntersection(t *testing.T) {
	handler := newHandler(t, testConfig(true))
	result := gatewayJSON(t, handler, "/api/v1/events/search", `{}`)
	if len(result["events"].([]any)) == 0 {
		t.Fatalf("event search returned no events: %v", result)
	}
}

func TestListSourcesUsesProjectAllowlistAndLiveStatus(t *testing.T) {
	cfg := testConfig(true)
	cfg.ProjectSources[projectID] = map[string]bool{"pt-sandbox": true}
	handler := newHandler(t, cfg)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	request.Header.Set("X-Project-ID", projectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Items []struct {
			Code   string `json:"code"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Code != "pt-sandbox" || result.Items[0].Status != "online" {
		t.Fatalf("items=%+v", result.Items)
	}
}

func TestListSourcesKeepsMockAccountSourceOnlineWithoutSecrets(t *testing.T) {
	handler := newHandler(t, testConfig(true))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	request.Header.Set("X-Project-ID", projectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Items []struct {
			Code   string `json:"code"`
			Mode   string `json:"mode"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		if item.Code != "maxpatrol-siem" {
			continue
		}
		if item.Mode != "mock" || item.Status != "online" {
			t.Fatalf("maxpatrol source=%+v", item)
		}

		userinfoRequest := httptest.NewRequest(http.MethodGet, "/api/v1/sources/maxpatrol-siem/account/userinfo", nil)
		userinfoRequest.Header.Set("X-Project-ID", projectID)
		userinfoResponse := httptest.NewRecorder()
		handler.ServeHTTP(userinfoResponse, userinfoRequest)
		if userinfoResponse.Code != http.StatusServiceUnavailable {
			t.Fatalf("userinfo status=%d body=%s", userinfoResponse.Code, userinfoResponse.Body.String())
		}

		search := gatewayJSON(t, handler, "/api/v1/events/search", `{"sources":["maxpatrol-siem"]}`)
		if len(search["events"].([]any)) == 0 {
			t.Fatal("mock MaxPatrol event provider returned no events")
		}
		return
	}
	t.Fatalf("maxpatrol-siem missing from sources: %+v", result.Items)
}

func TestMaxPatrolAccountUserinfoRequiresConfiguration(t *testing.T) {
	handler := newHandler(t, testConfig(true))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources/maxpatrol-siem/account/userinfo", nil)
	request.Header.Set("X-Project-ID", projectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSourceAuthenticationFailureIsBadGateway(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	resolver := &staticSecrets{values: map[string]string{
		"DEMO_PT_SIEM_BASE_URL": upstream.URL,
		"DEMO_PT_COOKIE":        "expired=1",
	}}
	providers, _, err := adaptermock.NewRegistry(adaptermock.Options{EventCount: 1, EndpointCount: 1, HistoryDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := httptransport.NewHandler(testConfig(true), log, service.New(providers, resolver, time.Second, time.Second, false))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources/maxpatrol-siem/account/userinfo", nil)
	request.Header.Set("Authorization", "Bearer platform-token")
	request.Header.Set("X-Project-ID", projectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"code":"source_auth_failed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if resolver.calls != 2 || upstreamCalls != 2 {
		t.Fatalf("resolver_calls=%d upstream_calls=%d", resolver.calls, upstreamCalls)
	}
}

func TestEventsExposeSourceOwnedIdentityAndResolveContext(t *testing.T) {
	handler := newHandler(t, testConfig(true))

	search := gatewayJSON(t, handler, "/api/v1/events/search", `{"sources":["maxpatrol-siem"]}`)
	events := search["events"].([]any)
	if len(events) == 0 {
		t.Fatal("event search returned no events")
	}
	event := events[0].(map[string]any)
	if event["source_code"] != "maxpatrol-siem" || event["source_event_id"] == "" {
		t.Fatalf("event has no source-owned identity: %v", event)
	}
	if _, exists := event["id"]; exists {
		t.Fatalf("event still exposes a computed id: %v", event)
	}

	body, _ := json.Marshal(map[string]any{
		"events":   []any{map[string]any{"source_code": event["source_code"], "source_event_id": event["source_event_id"]}},
		"entities": []any{},
	})
	resolved := gatewayJSON(t, handler, "/api/v1/context/resolve", string(body))
	resolvedEvents := resolved["events"].([]any)
	if len(resolvedEvents) != 1 || resolvedEvents[0].(map[string]any)["source_event_id"] != event["source_event_id"] {
		t.Fatalf("unexpected resolved context: %v", resolved)
	}
	for _, entityValue := range resolved["entities"].([]any) {
		entity := entityValue.(map[string]any)
		if _, exists := entity["id"]; exists {
			t.Fatalf("entity still exposes a computed id: %v", entity)
		}
		if len(entity["sources"].([]any)) == 0 {
			t.Fatalf("entity has no source reference: %v", entity)
		}
	}
	searchEntities := search["entities"].([]any)
	if len(searchEntities) == 0 {
		t.Fatal("event search returned no entity context")
	}
	source := searchEntities[0].(map[string]any)["sources"].([]any)[0].(map[string]any)
	body, _ = json.Marshal(map[string]any{
		"events":   []any{},
		"entities": []any{map[string]any{"source_code": source["source_code"], "source_entity_id": source["source_entity_id"]}},
	})
	entityOnly := gatewayJSON(t, handler, "/api/v1/context/resolve", string(body))
	if len(entityOnly["events"].([]any)) != 0 || len(entityOnly["entities"].([]any)) != 1 {
		t.Fatalf("unexpected entity-only context: %v", entityOnly)
	}

	missing := httptest.NewRequest(http.MethodPost, "/api/v1/context/resolve", bytes.NewBufferString(`{"events":[{"source_code":"maxpatrol-siem","source_event_id":"missing"}],"entities":[]}`))
	missing.Header.Set("Content-Type", "application/json")
	missing.Header.Set("X-Project-ID", projectID)
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing source record status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}
}

func TestEventSearchPivotsByCanonicalDestinationIP(t *testing.T) {
	handler := newHandler(t, testConfig(true))
	search := gatewayJSON(t, handler, "/api/v1/events/search", `{
		"sources":["maxpatrol-siem"],
		"entities":[{"type":"ip","value":"192.0.2.62"}],
		"limit":100
	}`)

	eventValues := search["events"].([]any)
	gotEventIDs := make([]string, len(eventValues))
	for index, eventValue := range eventValues {
		gotEventIDs[index] = eventValue.(map[string]any)["source_event_id"].(string)
	}
	wantEventIDs := []string{"ev-13", "ev-12", "ev-11"}
	if !reflect.DeepEqual(gotEventIDs, wantEventIDs) {
		t.Fatalf("event IDs=%v want=%v", gotEventIDs, wantEventIDs)
	}

	for _, entityValue := range search["entities"].([]any) {
		entity := entityValue.(map[string]any)
		if entity["type"] == "host" && entity["value"] == "ws-beta.corp.example" {
			return
		}
	}
	t.Fatalf("ws-beta host is missing from response entities: %v", search["entities"])
}

func gatewayJSON(t *testing.T, handler http.Handler, path, body string) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Project-ID", projectID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func newHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	providers, _, err := adaptermock.NewRegistry(adaptermock.Options{EventCount: 1, EndpointCount: 1, HistoryDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptransport.NewHandler(cfg, log, service.New(providers, nil, time.Second, time.Second, false))
}

type staticSecrets struct {
	values map[string]string
	calls  int
}

func (resolver *staticSecrets) Resolve(_ context.Context, bearer, projectID string, names ...string) (map[string]string, error) {
	resolver.calls++
	if bearer == "" || projectID == "" {
		return nil, errors.New("missing project access")
	}
	values := make(map[string]string, len(names))
	for _, name := range names {
		values[name] = resolver.values[name]
	}
	return values, nil
}

func testConfig(authDisabled bool) config.Config {
	return config.Config{
		Auth: config.AuthConfig{
			Disabled: authDisabled,
			Issuer:   "auth.local",
			Audience: "api.local",
			Kid:      "ed25519-v1",
			Typ:      "access+jwt",
		},
		ProjectSources: map[string]map[string]bool{
			projectID: {"maxpatrol-siem": true, "pt-sandbox": true},
		},
	}
}

func signToken(t *testing.T, privateKey ed25519.PrivateKey, cfg config.AuthConfig) string {
	t.Helper()
	claims := &coreauth.AccessTokenClaims{
		SessionID:   "session",
		SubjectKind: "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			Subject:   "subject",
			Audience:  jwt.ClaimStrings{cfg.Audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "token-id",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = cfg.Kid
	token.Header["typ"] = cfg.Typ
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
