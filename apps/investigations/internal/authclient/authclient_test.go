package authclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvestigationToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/agent-tokens/investigation" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-jwt" {
			t.Fatalf("unexpected authorization %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"agent-jwt","token_type":"Bearer","expires_in":14400}`))
	}))
	defer server.Close()

	token, err := New(Config{BaseURL: server.URL}).InvestigationToken(
		context.Background(), "user-jwt", "abcdef1234", "11111111-1111-1111-1111-111111111111")
	if err != nil || token != "agent-jwt" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestInvestigationTokenReturnsUpstreamStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := New(Config{BaseURL: server.URL}).InvestigationToken(
		context.Background(), "user-jwt", "abcdef1234", "11111111-1111-1111-1111-111111111111")
	upstream, ok := err.(*HTTPError)
	if !ok || upstream.Status != http.StatusForbidden {
		t.Fatalf("unexpected error %#v", err)
	}
}

func TestExchangeAccessToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/agent-tokens/exchange" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		clientID, secret, ok := r.BasicAuth()
		if !ok || clientID != "ir-api" || secret != "01234567890123456789012345678901" {
			t.Fatalf("unexpected client authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"short-access-jwt","token_type":"Bearer","expires_in":300}`))
	}))
	defer server.Close()

	token, err := New(Config{BaseURL: server.URL, ClientID: "ir-api", ClientSecret: "01234567890123456789012345678901"}).
		ExchangeAccessToken(context.Background(), "agent-jwt")
	if err != nil || token != "short-access-jwt" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}
