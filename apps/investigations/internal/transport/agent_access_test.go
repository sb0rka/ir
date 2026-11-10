package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
	"github.com/sb0rka/ir/packages/contract/authentication"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
	authctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"
)

const transportAgentProject = "aabbccddee"
const transportAgentSecretID = "00000000-0000-0000-0000-000000000001"

type transportAgentDatabase struct {
	store.AuthDatabase
	store.AgentAccessDatabase
	token       string
	err         error
	verifyCalls int
}

func (d *transportAgentDatabase) Verify(_ context.Context, project, raw string) (agentaccess.Record, error) {
	d.verifyCalls++
	if d.err != nil {
		return agentaccess.Record{}, d.err
	}
	if project != transportAgentProject || raw != d.token {
		return agentaccess.Record{}, agentaccess.ErrUnauthorized
	}
	return agentaccess.Record{ProjectID: project, SecretID: transportAgentSecretID, VersionNo: 1, DigestAlg: "sha256", Digest: agentaccess.Digest(raw)}, nil
}

func transportAgentFixture(t *testing.T, gatewayURL string) (*authflow.Service, config.Config, *transportAgentDatabase, *atomic.Int32) {
	t.Helper()
	userCalls := &atomic.Int32{}
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCalls.Add(1)
		http.Error(w, "unexpected user credential flow", http.StatusInternalServerError)
	}))
	t.Cleanup(userServer.Close)
	db := &transportAgentDatabase{token: agentaccess.Prefix + strings.Repeat("A", 52)}
	cfg := config.Config{
		Server: config.ServerConfig{Auth: config.AuthConfig{
			Issuer: userServer.URL, ClientID: coreauth.ClientIRWeb, WebURL: "http://ir-ui.example",
			SessionKey: bytes.Repeat([]byte{1}, 32), ContextKey: bytes.Repeat([]byte{2}, 32),
		}},
		Platform: config.PlatformConfig{APIBaseURL: userServer.URL},
		Gateway:  config.GatewayConfig{BaseURL: gatewayURL, ConfigurationKey: bytes.Repeat([]byte{3}, 32)},
	}
	flow, err := authflow.New(context.Background(), db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return flow, cfg, db, userCalls
}

func transportAgentRequest(method, path, token string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set(projectIDHeader, transportAgentProject)
	return r
}

func transportAgentLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestAgentMiddlewarePropagatesVerifiedProjectAndIdentity(t *testing.T) {
	flow, _, db, userCalls := transportAgentFixture(t, "http://gateway.example")
	for _, path := range []string{"/api/v1/investigations", "/mcp"} {
		t.Run(path, func(t *testing.T) {
			called := false
			handler := authMiddleware(flow, transportAgentLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				scope, ok := socctx.ScopeFromContext(r.Context())
				if !ok || scope.ProjectID != transportAgentProject {
					t.Error("verified project missing from context")
				}
				bearer, ok := socctx.BearerFromContext(r.Context())
				if !ok || bearer != db.token {
					t.Error("verified bearer missing from context")
				}
				identity, ok := authctx.IdentityFromContext(r.Context())
				if !ok || identity.SubjectID != transportAgentSecretID || identity.SubjectKind != coreauth.SubjectAgent || identity.SessionID != "" {
					t.Error("wrong agent identity propagated")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, transportAgentRequest(http.MethodGet, path, db.token))
			if response.Code != http.StatusNoContent || !called {
				t.Fatalf("agent rejected: %d", response.Code)
			}
		})
	}
	if userCalls.Load() != 0 {
		t.Fatal("agent request invoked Auth or Platform")
	}
}

func TestAgentMiddlewareRejectsInvalidCredentialsAndAmbiguousHeaders(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*http.Request, *transportAgentDatabase)
		status int
	}{
		{"wrong token", func(r *http.Request, _ *transportAgentDatabase) {
			r.Header.Set("Authorization", "Bearer "+agentaccess.Prefix+strings.Repeat("B", 52))
		}, http.StatusUnauthorized},
		{"foreign project", func(r *http.Request, _ *transportAgentDatabase) { r.Header.Set(projectIDHeader, "1122334455") }, http.StatusUnauthorized},
		{"storage unavailable", func(_ *http.Request, db *transportAgentDatabase) { db.err = errors.New("database unavailable") }, http.StatusServiceUnavailable},
		{"duplicate authorization", func(r *http.Request, db *transportAgentDatabase) { r.Header.Add("Authorization", "Bearer "+db.token) }, http.StatusUnauthorized},
		{"duplicate project", func(r *http.Request, _ *transportAgentDatabase) { r.Header.Add(projectIDHeader, "1122334455") }, http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			flow, _, db, userCalls := transportAgentFixture(t, "http://gateway.example")
			called := false
			handler := authMiddleware(flow, transportAgentLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
			r := transportAgentRequest(http.MethodGet, "/mcp", db.token)
			test.mutate(r, db)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != test.status || called {
				t.Fatalf("status=%d want=%d downstream_called=%v", response.Code, test.status, called)
			}
			if userCalls.Load() != 0 {
				t.Fatal("agent rejection invoked Auth or Platform")
			}
		})
	}
}

func TestAgentKeyCannotManageCredentialsOrConfiguration(t *testing.T) {
	flow, cfg, db, userCalls := transportAgentFixture(t, "http://gateway.example")
	h := NewAuthHandler(flow, cfg.Server.Auth, transportAgentLogger())
	for _, test := range []struct {
		name, method, path, body string
		handler                  http.HandlerFunc
	}{
		{"read access", http.MethodGet, "/api/v1/agent-access", "", func(w http.ResponseWriter, r *http.Request) {
			h.GetIRAgentAccess(w, r, authentication.GetIRAgentAccessParams{})
		}},
		{"create access", http.MethodPost, "/api/v1/agent-access", "", func(w http.ResponseWriter, r *http.Request) {
			h.CreateIRAgentAccess(w, r, authentication.CreateIRAgentAccessParams{})
		}},
		{"rotate access", http.MethodPost, "/api/v1/agent-access/rotate", `{"expected_version":1}`, func(w http.ResponseWriter, r *http.Request) {
			h.RotateIRAgentAccess(w, r, authentication.RotateIRAgentAccessParams{})
		}},
		{"read configuration", http.MethodGet, "/api/v1/configuration", "", func(w http.ResponseWriter, r *http.Request) {
			h.GetIRConfiguration(w, r, authentication.GetIRConfigurationParams{})
		}},
		{"write configuration", http.MethodPut, "/api/v1/configuration", `[]`, func(w http.ResponseWriter, r *http.Request) {
			h.PutIRConfiguration(w, r, authentication.PutIRConfigurationParams{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := transportAgentRequest(test.method, test.path, db.token)
			r.Header.Set("Origin", cfg.Server.Auth.WebURL)
			r.Body = io.NopCloser(strings.NewReader(test.body))
			response := httptest.NewRecorder()
			test.handler.ServeHTTP(response, r)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("agent managed user credentials: status=%d", response.Code)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Error("management rejection cacheable")
			}
		})
	}
	if db.verifyCalls != 0 || userCalls.Load() != 0 {
		t.Fatal("agent management attempted a privileged flow")
	}
}

func TestAgentGatewayProxyForwardsOnlyVerifiedBearerAndProject(t *testing.T) {
	var calls atomic.Int32
	var expectedToken string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/sources" || r.Header.Get("Authorization") != "Bearer "+expectedToken || r.Header.Get(projectIDHeader) != transportAgentProject {
			t.Error("Gateway received wrong target or credentials")
		}
		if r.Header.Get(irconfig.TokenHeader) != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("Proxy-Authorization") != "" {
			t.Error("untrusted downstream credentials forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sources":[]}`))
	}))
	defer gateway.Close()
	flow, _, db, userCalls := transportAgentFixture(t, gateway.URL)
	expectedToken = db.token
	handler := gatewayProxy(Dependencies{AuthFlow: flow, Log: transportAgentLogger()})
	r := transportAgentRequest(http.MethodGet, "/api/v1/gateway/api/v1/sources", db.token)
	r.Header.Set("Cookie", "ir_refresh=must-not-forward")
	r.Header.Set("Origin", "http://ir-ui.example")
	r.Header.Set("Proxy-Authorization", "Bearer injected")
	r.Header.Set(irconfig.TokenHeader, "Bearer injected-config")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	if response.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("status=%d upstream_calls=%d", response.Code, calls.Load())
	}
	if userCalls.Load() != 0 {
		t.Fatal("proxy used user refresh/exchange/reveal")
	}
}

func TestAgentGatewayProxyDoesNotForwardRejectedRequests(t *testing.T) {
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer gateway.Close()
	for _, test := range []struct {
		name, method, path string
		mutate             func(*http.Request)
		status             int
	}{
		{"unknown key", http.MethodGet, "/api/v1/gateway/api/v1/sources", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+agentaccess.Prefix+strings.Repeat("B", 52))
		}, http.StatusUnauthorized},
		{"foreign project", http.MethodGet, "/api/v1/gateway/api/v1/sources", func(r *http.Request) { r.Header.Set(projectIDHeader, "1122334455") }, http.StatusUnauthorized},
		{"duplicate authorization", http.MethodGet, "/api/v1/gateway/api/v1/sources", func(r *http.Request) { r.Header.Add("Authorization", "Bearer injected") }, http.StatusUnauthorized},
		{"duplicate project", http.MethodGet, "/api/v1/gateway/api/v1/sources", func(r *http.Request) { r.Header.Add(projectIDHeader, "1122334455") }, http.StatusUnauthorized},
		{"unlisted operation", http.MethodPost, "/api/v1/gateway/api/v1/arbitrary/admin", func(*http.Request) {}, http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			flow, _, db, userCalls := transportAgentFixture(t, gateway.URL)
			handler := gatewayProxy(Dependencies{AuthFlow: flow, Log: transportAgentLogger()})
			r := transportAgentRequest(test.method, test.path, db.token)
			test.mutate(r)
			response := httptest.NewRecorder()
			before := calls.Load()
			handler.ServeHTTP(response, r)
			if response.Code != test.status || calls.Load() != before {
				t.Fatalf("status=%d want=%d forwarded=%v", response.Code, test.status, calls.Load() != before)
			}
			if userCalls.Load() != 0 {
				t.Fatal("rejected agent request used user credentials")
			}
		})
	}
}
