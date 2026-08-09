package http_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
		roles     staticRoles
		projectID string
		status    int
	}{
		{name: "missing role", roles: staticRoles{}, projectID: projectID, status: http.StatusForbidden},
		{name: "unconfigured project", roles: staticRoles{allowed: true}, projectID: "aabbccddee", status: http.StatusForbidden},
		{name: "configured project", roles: staticRoles{allowed: true}, projectID: projectID, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler(t, cfg, test.roles)
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

func TestEventSearchEnforcesOpenAPIInputLimits(t *testing.T) {
	handler := newHandler(t, testConfig(true), nil)
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

type staticRoles struct{ allowed bool }

func (roles staticRoles) HasRoleBinding(context.Context, string, string) (bool, error) {
	return roles.allowed, nil
}

func newHandler(t *testing.T, cfg config.Config, roles httptransport.RoleResolver) http.Handler {
	t.Helper()
	providers, _, err := adaptermock.NewRegistry(adaptermock.Options{EventCount: 1, EndpointCount: 1, HistoryDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptransport.NewHandler(cfg, log, service.New(providers, time.Second, time.Second), roles)
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
