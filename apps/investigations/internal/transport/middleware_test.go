package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
	coreauthctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

func TestBearerToken(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"Bearer token", "bearer token", "  Bearer   token  "} {
		token, err := bearerToken(header)
		if err != nil || token != "token" {
			t.Fatalf("bearerToken(%q) = %q, %v", header, token, err)
		}
	}

	for _, header := range []string{"", "Bearer", "Basic token", "Bearer one two"} {
		if _, err := bearerToken(header); err == nil {
			t.Fatalf("bearerToken(%q) accepted malformed header", header)
		}
	}
}

func TestVerifyAccessToken(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.AuthConfig{
		AccessTokenPublicKey: publicKey,
		AccessTokenIssuer:    "auth.local",
		AccessTokenAudience:  "deployment-specific-audience",
		AccessTokenKid:       "ed25519-v1",
		AccessTokenTyp:       "access+jwt",
	}

	t.Run("valid", func(t *testing.T) {
		identity, err := verify(signAccessToken(t, privateKey, cfg, nil), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if identity.SubjectID != "subject" || identity.SubjectKind != "user" ||
			identity.JTI != "token-id" || identity.ClientID != "som" {
			t.Fatalf("unexpected identity: %#v", identity)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		token := signAccessToken(t, privateKey, cfg, func(claims *coreauth.AccessTokenClaims) {
			claims.Audience = jwt.ClaimStrings{"other.local"}
		})
		if _, err := verify(token, cfg); err == nil {
			t.Fatal("token with wrong audience accepted")
		}
	})

	t.Run("multiple audiences", func(t *testing.T) {
		token := signAccessToken(t, privateKey, cfg, func(claims *coreauth.AccessTokenClaims) {
			claims.Audience = jwt.ClaimStrings{cfg.AccessTokenAudience, "other.local"}
		})
		if _, err := verify(token, cfg); err == nil {
			t.Fatal("token with multiple audiences accepted")
		}
	})

	for _, field := range []string{"subject_kind", "jti"} {
		t.Run("missing "+field, func(t *testing.T) {
			token := signAccessToken(t, privateKey, cfg, func(claims *coreauth.AccessTokenClaims) {
				if field == "subject_kind" {
					claims.SubjectKind = ""
				} else {
					claims.ID = ""
				}
			})
			if _, err := verify(token, cfg); err == nil {
				t.Fatalf("token without %s accepted", field)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.ServerConfig{Auth: config.AuthConfig{
		AccessTokenPublicKey: publicKey,
		AccessTokenIssuer:    "auth.local",
		AccessTokenAudience:  "deployment-specific-audience",
		AccessTokenKid:       "ed25519-v1",
		AccessTokenTyp:       "access+jwt",
	}}
	token := signAccessToken(t, privateKey, cfg.Auth, nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("jwt is checked before project header", func(t *testing.T) {
		nextCalled := false
		handler := authMiddleware(cfg, log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/investigations", nil))
		if response.Code != http.StatusUnauthorized || nextCalled {
			t.Fatalf("status=%d nextCalled=%v", response.Code, nextCalled)
		}
	})

	t.Run("scope reaches handler", func(t *testing.T) {
		handler := authMiddleware(cfg, log)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, identityOK := coreauthctx.IdentityFromContext(r.Context())
				scope, scopeOK := socctx.ScopeFromContext(r.Context())
				bearer, bearerOK := socctx.BearerFromContext(r.Context())
				if !identityOK || !scopeOK || identity.SubjectID != "subject" ||
					identity.ClientID != "som" || scope.ProjectID != "aabbccddee" ||
					!bearerOK || bearer != token {
					t.Fatalf("missing auth context: identity=%#v scope=%#v", identity, scope)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
		request := httptest.NewRequest(http.MethodGet, "/api/v1/investigations", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set(projectIDHeader, "aabbccddee")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d", response.Code)
		}
	})

	for _, projectID := range []string{"", "ABCDEF1234", "short", "aabbccddee/other"} {
		t.Run("reject project "+projectID, func(t *testing.T) {
			nextCalled := false
			handler := authMiddleware(cfg, log)(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/investigations", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set(projectIDHeader, projectID)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest || nextCalled {
				t.Fatalf("project=%q status=%d nextCalled=%v", projectID, response.Code, nextCalled)
			}
		})
	}

	t.Run("disabled auth keeps project scope", func(t *testing.T) {
		disabled := cfg
		disabled.Auth.Disabled = true
		handler := authMiddleware(disabled, log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := socctx.ScopeFromContext(r.Context())
			if !ok || scope.ProjectID != "aabbccddee" {
				t.Fatalf("scope=%#v ok=%v", scope, ok)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		request := httptest.NewRequest(http.MethodGet, "/api/v1/investigations", nil)
		request.Header.Set(projectIDHeader, "aabbccddee")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d", response.Code)
		}
	})
}

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

func signAccessToken(
	t *testing.T,
	privateKey ed25519.PrivateKey,
	cfg config.AuthConfig,
	change func(*coreauth.AccessTokenClaims),
) string {
	t.Helper()
	claims := &coreauth.AccessTokenClaims{
		SessionID:   "session",
		SubjectKind: "user",
		ClientID:    "som",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.AccessTokenIssuer,
			Subject:   "subject",
			Audience:  jwt.ClaimStrings{cfg.AccessTokenAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "token-id",
		},
	}
	if change != nil {
		change(claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = cfg.AccessTokenKid
	token.Header["typ"] = cfg.AccessTokenTyp
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
