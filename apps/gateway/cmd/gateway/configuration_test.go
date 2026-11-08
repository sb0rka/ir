package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	gatewayproxy "github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

func TestJSONConfigurationReachesVendorOnlyAsSelectedCookie(t *testing.T) {
	var version atomic.Int32
	version.Store(1)
	var reads, vendorCalls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer api-read-token" {
			t.Error("unexpected configuration request")
		}
		if r.URL.Path == "/projects/aabbccddee/secrets" {
			json.NewEncoder(w).Encode(map[string]any{"secrets": []any{map[string]any{"name": "IR/SOURCE_CONFIGURATION", "public_id": "012345abcdef", "secret_id": "fixture-secret", "payload_kind": "json", "current_version_no": version.Load()}}})
			return
		}
		if r.URL.Path != fmt.Sprintf("/projects/aabbccddee/resources/012345abcdef/secret/versions/%d/reveal", version.Load()) {
			t.Error("unexpected generic Secrets operation", r.URL.Path)
		}
		reads.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"secret_id": "fixture-secret", "version_no": version.Load(), "value": `[{"name":"pt-maxpatrol-siem","CorePortalCookie":"core","idsrv.session":"session","idsrv":"identity","IncidentManagementPortalCookie":"portal","UnrelatedSecret":"never-forward"},{"name":"pt-nad","CorePortalCookie":"wrong-source","sessionid":"nad","csrftoken":"csrf"}]`})
	}))
	defer api.Close()
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vendorCalls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get(irconfig.TokenHeader) != "" || r.Header.Get("Cookie") != "CorePortalCookie=core; idsrv.session=session; idsrv=identity; IncidentManagementPortalCookie=portal" {
			t.Error("vendor received incorrect credentials")
		}
		if r.URL.Path != "/api/account/userinfo" {
			t.Error("unexpected vendor operation", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"userName":"fixture-user"}`))
	}))
	defer vendor.Close()
	provider, err := maxpatrol.NewProvider(maxpatrol.ClientConfig{SIEM: gatewayproxy.HTTPClientConfig{BaseURL: vendor.URL, Timeout: time.Second}, Incidents: gatewayproxy.HTTPClientConfig{BaseURL: vendor.URL, Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	client, err := common.NewConfigurationClient(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(reg, commonSecretResolver{client: client}, time.Second, time.Second)
	ctx := common.WithConfigurationSnapshot(context.Background())
	available := svc.AvailableSources(ctx, service.ProjectAccess{ProjectID: "aabbccddee", Bearer: "api-read-token"})
	if len(available) != 1 || available[0] != "pt-maxpatrol-siem" || vendorCalls.Load() != 0 {
		t.Fatal("availability did not follow project JSON or contacted the vendor")
	}
	for range 2 {
		result, err := svc.GetAccountUserinfo(ctx, service.ProjectAccess{ProjectID: "aabbccddee", Bearer: "api-read-token"}, "pt-maxpatrol-siem")
		if err != nil || result.UserName != "fixture-user" {
			t.Fatal("vendor fixture failed", err)
		}
	}
	if reads.Load() != 1 || vendorCalls.Load() != 2 {
		t.Fatal("request snapshot not shared", reads.Load(), vendorCalls.Load())
	}
	version.Store(2)
	if _, err := svc.GetAccountUserinfo(common.WithConfigurationSnapshot(context.Background()), service.ProjectAccess{ProjectID: "aabbccddee", Bearer: "api-read-token"}, "pt-maxpatrol-siem"); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 2 {
		t.Fatal("plaintext configuration was reused across requests")
	}
}
