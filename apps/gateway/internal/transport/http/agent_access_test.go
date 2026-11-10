package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

type agentRepositoryFixture struct {
	token, project                  string
	verifyError, configurationError error
	verifyCalls, configurationCalls int
}

func (fixture *agentRepositoryFixture) Verify(_ context.Context, project, raw string) (agentaccess.Record, error) {
	fixture.verifyCalls++
	if fixture.verifyError != nil {
		return agentaccess.Record{}, fixture.verifyError
	}
	if project != fixture.project || raw != fixture.token {
		return agentaccess.Record{}, agentaccess.ErrUnauthorized
	}
	return agentaccess.Record{}, nil
}

func (fixture *agentRepositoryFixture) Configuration(_ context.Context, project string) (agentaccess.Configuration, error) {
	fixture.configurationCalls++
	return agentaccess.Configuration{ProjectID: project}, fixture.configurationError
}

type agentCipherFixture struct {
	err   error
	calls int
}

func (fixture *agentCipherFixture) Open(stored agentaccess.Configuration) (irconfig.Document, error) {
	fixture.calls++
	return irconfig.Document{Value: "[]", Values: map[string]map[string]string{"pt-nad": {"sessionid": stored.ProjectID, "csrftoken": "csrf"}}}, fixture.err
}

func TestAgentGatewayUsesRequestConfigurationWithoutUserCredentials(t *testing.T) {
	token := "s0_ir_agent_" + strings.Repeat("A", 52)
	repository := &agentRepositoryFixture{token: token, project: "aabbccddee"}
	cipher := &agentCipherFixture{}
	client, err := common.NewConfigurationClient("https://platform.invalid")
	if err != nil {
		t.Fatal(err)
	}
	nextCalls := 0
	next := projectScope()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		access := projectAccess(r)
		if access.ProjectID != repository.project || access.Bearer != token {
			t.Fatalf("agent access was not propagated: project=%q", access.ProjectID)
		}
		document, err := client.Snapshot(r.Context(), access.Bearer, access.ProjectID)
		if err != nil || document.Values["pt-nad"]["sessionid"] != repository.project {
			t.Fatalf("server configuration not available: %v", err)
		}
		if _, err := client.Snapshot(r.Context(), access.Bearer, "1122334455"); err == nil {
			t.Fatal("configuration escaped project binding")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	handler := gatewayAuthentication(config.AuthConfig{}, &AgentAccess{Repository: repository, Cipher: cipher})(next)
	request := func(raw, project string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set(projectIDHeader, project)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for range 2 {
		if w := request(token, repository.project); w.Code != http.StatusNoContent || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("agent request failed: %d %s", w.Code, w.Body.String())
		}
	}
	if repository.verifyCalls != 2 || repository.configurationCalls != 2 || cipher.calls != 2 {
		t.Fatal("agent requests reused authorization or plaintext across requests")
	}
	repository.token = "s0_ir_agent_" + strings.Repeat("B", 52)
	if w := request(token, repository.project); w.Code != http.StatusUnauthorized {
		t.Fatal("rotated key remained valid", w.Code)
	}
	if w := request(repository.token, "1122334455"); w.Code != http.StatusUnauthorized {
		t.Fatal("foreign project accepted", w.Code)
	}
	if nextCalls != 2 || repository.configurationCalls != 2 {
		t.Fatal("unauthorized request reached configuration or handler")
	}
}

func TestAgentGatewayFailsClosed(t *testing.T) {
	token := "s0_ir_agent_" + strings.Repeat("A", 52)
	for _, test := range []struct {
		name   string
		mutate func(*http.Request, *agentRepositoryFixture, *agentCipherFixture)
		status int
	}{
		{"missing project", func(r *http.Request, _ *agentRepositoryFixture, _ *agentCipherFixture) { r.Header.Del(projectIDHeader) }, 401},
		{"duplicate project", func(r *http.Request, _ *agentRepositoryFixture, _ *agentCipherFixture) {
			r.Header.Add(projectIDHeader, "aabbccddee")
		}, 401},
		{"duplicate bearer", func(r *http.Request, _ *agentRepositoryFixture, _ *agentCipherFixture) {
			r.Header.Add("Authorization", "Bearer "+token)
		}, 401},
		{"malformed token", func(r *http.Request, _ *agentRepositoryFixture, _ *agentCipherFixture) {
			r.Header.Set("Authorization", "Bearer s0_ir_agent_short")
		}, 401},
		{"invalid key", func(_ *http.Request, repository *agentRepositoryFixture, _ *agentCipherFixture) {
			repository.verifyError = agentaccess.ErrUnauthorized
		}, 401},
		{"database down", func(_ *http.Request, repository *agentRepositoryFixture, _ *agentCipherFixture) {
			repository.verifyError = errors.New("database unavailable")
		}, 503},
		{"missing configuration", func(_ *http.Request, repository *agentRepositoryFixture, _ *agentCipherFixture) {
			repository.configurationError = agentaccess.ErrConfigurationMissing
		}, 503},
		{"unreadable configuration", func(_ *http.Request, _ *agentRepositoryFixture, cipher *agentCipherFixture) {
			cipher.err = errors.New("decryption failed")
		}, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &agentRepositoryFixture{token: token, project: "aabbccddee"}
			cipher := &agentCipherFixture{}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set(projectIDHeader, repository.project)
			test.mutate(r, repository, cipher)
			handler := gatewayAuthentication(config.AuthConfig{}, &AgentAccess{Repository: repository, Cipher: cipher})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed authorization reached handler") }))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), token) {
				t.Fatal("response exposed bearer")
			}
		})
	}
}

func TestAgentGatewayRequiresConfiguredStorage(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil)
	r.Header.Set("Authorization", "Bearer s0_ir_agent_"+strings.Repeat("A", 52))
	r.Header.Set(projectIDHeader, "aabbccddee")
	w := httptest.NewRecorder()
	gatewayAuthentication(config.AuthConfig{}, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unconfigured agent access reached handler") })).ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal(w.Code, w.Body.String())
	}
}
