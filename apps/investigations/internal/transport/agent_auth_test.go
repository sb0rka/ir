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

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

func TestVerifyAgentToken(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.AuthConfig{
		AccessTokenPublicKey: publicKey,
		AccessTokenIssuer:    "auth.local",
		AccessTokenKid:       "ed25519-v1",
	}

	t.Run("valid", func(t *testing.T) {
		claims, err := verifyAgentToken(signAgentToken(t, privateKey, cfg, nil), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if claims.ProjectID != "abcdef1234" || claims.InvestigationID != "11111111-1111-1111-1111-111111111111" {
			t.Fatalf("unexpected claims: %#v", claims)
		}
	})

	for name, change := range map[string]func(*investigationAgentClaims){
		"wrong audience": func(claims *investigationAgentClaims) { claims.Audience = jwt.ClaimStrings{"api.local"} },
		"multiple audiences": func(claims *investigationAgentClaims) {
			claims.Audience = jwt.ClaimStrings{agentTokenAudience, "api.local"}
		},
		"wrong issuer": func(claims *investigationAgentClaims) { claims.Issuer = "other.local" },
		"expired": func(claims *investigationAgentClaims) {
			claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
		},
		"missing expiry":      func(claims *investigationAgentClaims) { claims.ExpiresAt = nil },
		"missing scope":       func(claims *investigationAgentClaims) { claims.Scope = "" },
		"wrong investigation": func(claims *investigationAgentClaims) { claims.InvestigationID = "not-a-uuid" },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyAgentToken(signAgentToken(t, privateKey, cfg, change), cfg); err == nil {
				t.Fatalf("%s token accepted", name)
			}
		})
	}
	if _, err := verifyAgentToken(signAgentTokenHeader(t, privateKey, cfg, agentTokenType, "wrong-key", nil), cfg); err == nil {
		t.Fatal("wrong kid token accepted")
	}
	if _, err := verifyAgentToken(signAgentTokenHeader(t, privateKey, cfg, "wrong+jwt", cfg.AccessTokenKid, nil), cfg); err == nil {
		t.Fatal("wrong typ token accepted")
	}
}

func TestMCPAuthChecksAgentTokenWhenHumanAuthIsDisabled(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.ServerConfig{Auth: config.AuthConfig{
		Disabled:             true,
		AccessTokenPublicKey: publicKey,
		AccessTokenIssuer:    "auth.local",
		AccessTokenKid:       "ed25519-v1",
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, scopeOK := socctx.ScopeFromContext(r.Context())
		authorization, agentOK := socctx.AgentAuthorizationFromContext(r.Context())
		if !scopeOK || !agentOK || scope.ProjectID != "abcdef1234" {
			t.Fatalf("agent context missing: scope=%#v agent=%v", scope, agentOK)
		}
		if _, forwarded := socctx.BearerFromContext(r.Context()); forwarded {
			t.Fatal("agent JWT was exposed as an upstream bearer")
		}
		if authorization.Token == "" {
			t.Fatal("agent JWT was not retained in the dedicated exchange context")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mcpAuthMiddleware(cfg, log)(next)

	valid := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	valid.Header.Set("Authorization", "Bearer "+signAgentToken(t, privateKey, cfg.Auth, nil))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, valid)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid agent status=%d body=%s", response.Code, response.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	invalid.Header.Set("Authorization", "Bearer "+signAgentToken(t, privateKey, cfg.Auth, func(claims *investigationAgentClaims) {
		claims.Audience = jwt.ClaimStrings{"api.local"}
	}))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalid)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid agent status=%d body=%s", response.Code, response.Body.String())
	}

	wrongType := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	wrongType.Header.Set("Authorization", "Bearer "+signAgentTokenHeader(t, privateKey, cfg.Auth, "wrong+jwt", cfg.Auth.AccessTokenKid, nil))
	wrongType.Header.Set(projectIDHeader, "abcdef1234")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, wrongType)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-typ agent status=%d body=%s", response.Code, response.Body.String())
	}

	rest := authMiddleware(cfg, log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("agent JWT reached REST handler while AUTH_DISABLED")
	}))
	restRequest := httptest.NewRequest(http.MethodGet, "/api/v1/investigations", nil)
	restRequest.Header.Set("Authorization", "Bearer "+signAgentToken(t, privateKey, cfg.Auth, nil))
	restRequest.Header.Set(projectIDHeader, "abcdef1234")
	response = httptest.NewRecorder()
	rest.ServeHTTP(response, restRequest)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("agent JWT on REST status=%d body=%s", response.Code, response.Body.String())
	}
}

func signAgentToken(t *testing.T, privateKey ed25519.PrivateKey, cfg config.AuthConfig, change func(*investigationAgentClaims)) string {
	return signAgentTokenHeader(t, privateKey, cfg, agentTokenType, cfg.AccessTokenKid, change)
}

func signAgentTokenHeader(t *testing.T, privateKey ed25519.PrivateKey, cfg config.AuthConfig, typ, kid string, change func(*investigationAgentClaims)) string {
	t.Helper()
	now := time.Now().UTC()
	claims := investigationAgentClaims{
		SessionID:       "session",
		SubjectKind:     "user",
		ClientID:        "som",
		ProjectID:       "abcdef1234",
		InvestigationID: "11111111-1111-1111-1111-111111111111",
		Scope:           "investigation.graph.read investigation.events.read investigation.agent_results.write",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "subject", Issuer: cfg.AccessTokenIssuer,
			Audience:  jwt.ClaimStrings{agentTokenAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), IssuedAt: jwt.NewNumericDate(now), ID: "jti",
		},
	}
	if change != nil {
		change(&claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid
	token.Header["typ"] = typ
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}
