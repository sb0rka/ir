package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	"github.com/sb0rka/ir/packages/common"
)

type projectConfigurationFixture struct {
	mu     sync.RWMutex
	values map[string]map[string]map[string]string
}

func (fixture *projectConfigurationFixture) Resolve(_ context.Context, _ string, project string, names ...string) (map[string]string, error) {
	fixture.mu.RLock()
	defer fixture.mu.RUnlock()
	return common.ConfigurationCookies(fixture.values[project], names...)
}

func (fixture *projectConfigurationFixture) set(project string, values map[string]map[string]string) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.values[project] = values
}

type availabilityProvider struct {
	code, url string
	client    *http.Client
}

func (provider availabilityProvider) call(ctx context.Context, access capability.Access) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.url+"/"+provider.code, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Cookie", access.Cookie)
	response, err := provider.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("fixture unavailable")
	}
	return nil
}

func (provider availabilityProvider) Probe(ctx context.Context, access capability.Access) (string, error) {
	return "online", provider.call(ctx, access)
}

func (provider availabilityProvider) GetAccountUserinfo(ctx context.Context, access capability.Access) (domain.AccountUserinfo, error) {
	err := provider.call(ctx, access)
	return domain.AccountUserinfo{SourceCode: provider.code, UserName: "fixture-user"}, err
}

func TestSourceAvailabilityFollowsProjectCredentialsWithoutMapping(t *testing.T) {
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-IR-Config-Token") != "" {
			t.Error("vendor received an internal bearer")
		}
		cookie := r.Header.Get("Cookie")
		if cookie == "" || strings.Contains(cookie, "unrelated") || strings.Contains(cookie, "BASE_URL") {
			t.Error("vendor received invalid credentials")
		}
		w.WriteHeader(200)
	}))
	defer vendor.Close()
	providers := []registry.Provider{}
	for code, selector := range map[string]string{config.PTMaxPatrolSIEM: config.PTMaxPatrolCookie, config.PTNAD: config.PTNADCookie} {
		provider := availabilityProvider{code: code, url: vendor.URL, client: vendor.Client()}
		providers = append(providers, registry.Provider{Source: domain.Source{Code: code, Capabilities: []domain.Capability{domain.CapabilityAccountUserinfo}}, CredentialSecret: selector, AccountUserinfo: provider, Prober: provider})
	}
	reg, err := registry.New(providers...)
	if err != nil {
		t.Fatal(err)
	}
	credentials := &projectConfigurationFixture{values: map[string]map[string]map[string]string{
		"aabbccddee": {config.PTMaxPatrolSIEM: {"CorePortalCookie": "core", "idsrv.session": "session", "idsrv": "identity", "unrelated": "never-forward", "SOURCE_PT_NAD_BASE_URL": "https://ignored.invalid"}},
		"1122334455": {config.PTNAD: {"sessionid": "nad", "csrftoken": "csrf"}},
	}}
	svc := service.New(reg, credentials, time.Second, time.Second)
	handler := apiHandler(newGatewayServer(config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), svc))
	list := func(project string, want ...string) {
		t.Helper()
		response := exportRequest(t, handler, http.MethodGet, "/api/v1/sources", project, "")
		if response.Code != 200 {
			t.Fatalf("source listing failed: %d", response.Code)
		}
		var body struct {
			Items []api.Source `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, source := range body.Items {
			got = append(got, source.Code)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("project %s: got %v want %v", project, got, want)
		}
	}
	list("aabbccddee", config.PTMaxPatrolSIEM)
	list("1122334455", config.PTNAD)
	before := calls.Load()
	list("ffeeddccbb")
	if calls.Load() != before {
		t.Fatal("missing project credentials reached a vendor")
	}
	for _, source := range []string{config.PTNAD, "unregistered"} {
		response := exportRequest(t, handler, http.MethodGet, "/api/v1/sources/"+source+"/account/userinfo", "aabbccddee", "")
		if response.Code != 403 {
			t.Fatal("unavailable source accepted", response.Code)
		}
	}
	if calls.Load() != before {
		t.Fatal("unavailable source reached a vendor")
	}
	credentials.set("aabbccddee", map[string]map[string]string{config.PTNAD: {"sessionid": "new", "csrftoken": "csrf"}})
	list("aabbccddee", config.PTNAD)
	before = calls.Load()
	response := exportRequest(t, handler, http.MethodGet, "/api/v1/sources/"+config.PTMaxPatrolSIEM+"/account/userinfo", "aabbccddee", "")
	if response.Code != 403 || calls.Load() != before {
		t.Fatal("removed credentials retained source access")
	}
	credentials.set("aabbccddee", map[string]map[string]string{config.PTNAD: {"sessionid": "invalid; injected=x", "csrftoken": "csrf"}})
	list("aabbccddee")
	if calls.Load() != before {
		t.Fatal("invalid credentials reached a vendor or reused cached online status")
	}
	credentials.set("aabbccddee", map[string]map[string]string{})
	list("aabbccddee")
	list("1122334455", config.PTNAD)
}
