package common

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sb0rka/ir/packages/common/irconfig"
)

func TestConfigurationJSONContract(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `[null]`, `[[]]`, `[{}]`, `[{"name":""}]`, `[{"name":"  "}]`, `[{"name":1}]`, `[{"name":"pt-nad","a":null}]`, `[{"name":"pt-nad","a":1}]`, `[{"name":"pt-nad","a":{}}]`, `[{"name":"pt-nad","a":[]}]`, `[{"name":"pt-nad","a":"x","\u0061":"y"}]`, `[{"name":"pt-nad","\u006eame":"other"}]`, `[{"name":"pt-nad"},{"name":"pt-\u006ead"}]`, `[{"name":"pt-nad"},]`, `[{"name":"pt-nad",}]`, `[{"name":"pt-nad"}] {}`} {
		if _, err := irconfig.Parse([]byte(raw)); err == nil {
			t.Errorf("accepted invalid source array %s", raw)
		}
	}
	values, err := irconfig.Parse([]byte(`[{"name":"pt-maxpatrol-siem"," CorePortalCookie ":"  exact value  ","__proto__":"preserved"},{"name":"pt-nad","sessionid":"nad"}]`))
	if err != nil || values["pt-maxpatrol-siem"][" CorePortalCookie "] != "  exact value  " || values["pt-maxpatrol-siem"]["__proto__"] != "preserved" || values["pt-nad"]["sessionid"] != "nad" {
		t.Fatal("string contents changed", err)
	}
	if empty, err := irconfig.Parse([]byte(`[]`)); err != nil || len(empty) != 0 {
		t.Fatal("empty source array rejected", err)
	}
	prefix, suffix := `[{"name":"pt-nad","a":"`, `"}]`
	for _, size := range []int{irconfig.MaxBytes - len(prefix) - len(suffix), irconfig.MaxBytes - len(prefix) - len(suffix) + 1} {
		raw := []byte(prefix + strings.Repeat("x", size) + suffix)
		_, err := irconfig.Parse(raw)
		if len(raw) == irconfig.MaxBytes && err != nil {
			t.Fatal("exact size limit rejected", err)
		}
		if len(raw) > irconfig.MaxBytes && err == nil {
			t.Fatal("size limit ignored")
		}
	}

}

func TestConfigurationSnapshotIsRequestBoundAndConcurrent(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer config-read" {
			t.Error("wrong configuration context")
		}
		if r.URL.Path == "/projects/aabbccddee/secrets" {
			io.WriteString(w, `{"secrets":[{"public_id":"012345abcdef","secret_id":"fixture-secret","name":"IR/SOURCE_CONFIGURATION","payload_kind":"json","current_version_no":1}]}`)
			return
		}
		if r.URL.Path != "/projects/aabbccddee/resources/012345abcdef/secret/versions/1/reveal" {
			t.Error("unexpected generic Secrets operation", r.URL.Path)
		}
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"secret_id": "fixture-secret", "version_no": 1, "value": `[{"name":"pt-nad","sessionid":"session","csrftoken":"csrf","unrelated":"never forwarded"}]`})
	}))
	defer server.Close()
	client, err := NewConfigurationClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithConfigurationSnapshot(context.Background())
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.Snapshot(ctx, "config-read", "aabbccddee"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("one request fetched multiple snapshots", calls.Load())
	}
	document, err := client.Snapshot(WithConfigurationSnapshot(context.Background()), "config-read", "aabbccddee")
	if err != nil || calls.Load() != 2 {
		t.Fatal("configuration leaked between requests", err, calls.Load())
	}
	cookies, err := ConfigurationCookies(document.Values, "DEMO_PT_NAD_COOKIE")
	if err != nil || cookies["DEMO_PT_NAD_COOKIE"] != "sessionid=session; csrftoken=csrf" {
		t.Fatal(cookies, err)
	}
	if _, err := client.Snapshot(ctx, "config-read", "1122334455"); err == nil {
		t.Fatal("snapshot crossed project boundary")
	}
}

func TestConfigurationCookieSelection(t *testing.T) {
	valid := map[string]map[string]string{
		"pt-maxpatrol-siem": {"CorePortalCookie": "core", "idsrv.session": "session", "idsrv": "id", "IncidentManagementPortalCookie": "portal", "sessionid": "wrong-source", "csrftoken": "wrong-source", "Authorization": "must-not-forward"},
		"pt-nad":            {"sessionid": "nad", "csrftoken": "csrf", "CorePortalCookie": "wrong-source"},
	}
	cookies, err := ConfigurationCookies(valid, "DEMO_PT_SIEM_COOKIE", "DEMO_PT_NAD_COOKIE")
	if err != nil || cookies["DEMO_PT_SIEM_COOKIE"] != "CorePortalCookie=core; idsrv.session=session; idsrv=id; IncidentManagementPortalCookie=portal" || cookies["DEMO_PT_NAD_COOKIE"] != "sessionid=nad; csrftoken=csrf" {
		t.Fatal(cookies, err)
	}
	for _, bad := range []string{"x; injected=y", "x\r\nInjected: y", "x y", "\"quoted\"", " x", "x ", "\tx"} {
		valid["pt-maxpatrol-siem"]["CorePortalCookie"] = bad
		if _, err := ConfigurationCookies(valid, "DEMO_PT_SIEM_COOKIE"); err == nil {
			t.Fatal("invalid cookie accepted")
		}
	}
	if _, err := ConfigurationCookies(map[string]map[string]string{"pt-nad": {"sessionid": "only"}}, "DEMO_PT_NAD_COOKIE"); err == nil {
		t.Fatal("partial NAD credentials accepted")
	}
	if _, err := ConfigurationCookies(map[string]map[string]string{"pt-maxpatrol-siem": {"sessionid": "session", "csrftoken": "csrf"}}, "DEMO_PT_NAD_COOKIE"); err == nil {
		t.Fatal("credentials from another source accepted")
	}
}
