package gatewayclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

func TestAgentGatewayClientPreservesBearerWithoutConfigurationToken(t *testing.T) {
	raw, err := agentaccess.Generate()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+raw || r.Header.Get("X-Project-ID") != "aabbccddee" {
			t.Error("agent scope not forwarded")
		}
		if _, exists := r.Header[irconfig.TokenHeader]; exists || r.Header.Get(irconfig.TokenHeader) != "" {
			t.Error("configuration token forwarded on agent request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer upstream.Close()
	request, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+raw)
	request.Header.Set("X-Project-ID", "aabbccddee")
	request.Header.Set(irconfig.TokenHeader, "Bearer injected")
	doer := responseLimitDoer{next: upstream.Client(), resolver: evidenceTestBearer,
		configResolver: func(_ context.Context, bearer, project string) (string, error) {
			if bearer != raw || project != "aabbccddee" {
				t.Fatal("configuration resolver received wrong scope")
			}
			return "", nil
		}}
	response, err := doer.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if calls != 1 {
		t.Fatal("upstream not called")
	}
	doer.resolver = func(context.Context, string, string) (string, error) { return "", errors.New("revoked") }
	if _, err = doer.Do(request); err == nil || calls != 1 {
		t.Fatal("revoked key reached upstream")
	}
}

func TestUserGatewayClientStillSendsConfigurationToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer delegated-user" || r.Header.Get(irconfig.TokenHeader) != "Bearer platform-config" {
			t.Error("user delegation changed")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	request, _ := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer user")
	doer := responseLimitDoer{next: upstream.Client(), resolver: func(context.Context, string, string) (string, error) {
		return "delegated-user", nil
	}, configResolver: func(context.Context, string, string) (string, error) { return "platform-config", nil }}
	response, err := doer.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}
