package httptransport

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/ptnad"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

type nadFilterSecrets struct{}

func (nadFilterSecrets) Resolve(context.Context, string, string, ...string) (map[string]string, error) {
	return map[string]string{ptnad.CredentialSecretName: "csrftoken=test; sessionid=test"}, nil
}

func TestInvalidNADFilterReturnsClientErrorBeforeVendorCall(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid filter reached NAD")
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	client, err := ptnad.NewClient(ptnad.Config{BaseURL: upstream.URL, HTTPClient: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := ptnad.NewProvider(client, []int64{19, 23})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New(adapter.RegistryProvider())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(config.Config{Auth: config.AuthConfig{Disabled: true}, ProjectSources: map[string]map[string]bool{"aabbccddee": {"pt-nad": true}}}, slog.New(slog.NewTextHandler(io.Discard, nil)), service.New(reg, nadFilterSecrets{}, time.Second, time.Second))
	for _, kind := range []string{"findings", "sessions"} {
		response := exportRequest(t, handler, "POST", "/api/v1/"+kind+"/search", "aabbccddee", `{"sources":["pt-nad"],"filter":"(host.port == 2222","time_range":{"from":"2023-06-07T21:00:00Z","to":"2023-06-08T20:59:59Z"}}`)
		if response.Code != 400 || !strings.Contains(response.Body.String(), "invalid_request") || !strings.Contains(response.Body.String(), "NAD filter") {
			t.Fatalf("%s: HTTP %d %s", kind, response.Code, response.Body.String())
		}
	}
}
