package wazuh

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

func TestNewProviderUsesProcessCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"green"}`))
	}))
	defer server.Close()

	provider, err := NewProvider(ClientConfig{
		HTTP:         proxy.HTTPClientConfig{BaseURL: server.URL, Timeout: time.Second, SkipTLSVerify: true},
		Username:     "admin",
		Password:     "secret",
		IndexPattern: "wazuh-alerts-*",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.CredentialMode != registry.CredentialModeProcess {
		t.Fatalf("mode = %q", provider.CredentialMode)
	}
	if provider.CredentialSecret != "" {
		t.Fatal("process mode must not set credential secret")
	}
}

func TestSearchEventsBasicAuthAndPagination(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			t.Errorf("basic auth missing or wrong")
		}
		if strings.Contains(r.URL.Path, "_search") {
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			bodies = append(bodies, body)
			if len(bodies) == 1 {
				_, _ = w.Write([]byte(searchPageJSON("wazuh-alerts-4.x-sample-security", "doc-1", "2026-09-05T10:00:00.000Z")))
				return
			}
			_, _ = w.Write([]byte(searchPageJSON("wazuh-alerts-4.x-sample-security", "doc-2", "2026-09-05T09:00:00.000Z")))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := mustProvider(t, server.URL)
	page, err := provider.Events.SearchEvents(context.Background(), capability.Access{}, capability.SearchEventsRequest{
		TimeFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		TimeTo:   time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
		Limit:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.NextCursor == "" || page.Total == nil || *page.Total != 2 {
		t.Fatalf("page = %#v", page)
	}
	if page.Events[0].Provenance.ExternalID != "wazuh-alerts-4.x-sample-security/doc-1" {
		t.Fatalf("id = %q", page.Events[0].Provenance.ExternalID)
	}
	page2, err := provider.Events.SearchEvents(context.Background(), capability.Access{}, capability.SearchEventsRequest{
		TimeFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		TimeTo:   time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
		Limit:    1,
		Cursor:   page.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Events) != 1 {
		t.Fatalf("page2 = %#v", page2)
	}
	if page2.Events[0].Provenance.ExternalID != "wazuh-alerts-4.x-sample-security/doc-2" {
		t.Fatalf("id = %q", page2.Events[0].Provenance.ExternalID)
	}
	if _, ok := bodies[0]["search_after"]; ok {
		t.Fatal("first page must not send search_after")
	}
	if _, ok := bodies[1]["search_after"]; !ok {
		t.Fatal("second page must send search_after")
	}
}

func TestResolveRejectsForeignIndex(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	provider := mustProvider(t, server.URL)
	_, err := provider.Events.ResolveContext(context.Background(), capability.Access{}, capability.ResolveContextRequest{
		EventIDs: []string{"wazuh-archives-1/doc"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var requestErr *domain.RequestError
	if !asRequestError(err, &requestErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveReturnsEvent(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok || user != "admin" {
			t.Fatal("missing auth")
		}
		_, _ = w.Write([]byte(`{
			"_index":"wazuh-alerts-4.x-sample-security","_id":"doc-1","found":true,
			"_source":` + alertSourceJSON("sshd") + `
		}`))
	}))
	defer server.Close()
	provider := mustProvider(t, server.URL)
	page, err := provider.Events.ResolveContext(context.Background(), capability.Access{}, capability.ResolveContextRequest{
		EventIDs: []string{"wazuh-alerts-4.x-sample-security/doc-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].Type != "authentication.failure" {
		t.Fatalf("%#v", page.Events)
	}
}

func TestMapperFamilies(t *testing.T) {
	cases := map[string]struct {
		source   string
		wantType string
		wantSev  string
		freq     bool
	}{
		"sshd":      {alertSourceJSON("sshd"), "authentication.failure", "medium", true},
		"syscheck":  {alertSourceJSON("syscheck"), "file.integrity", "medium", false},
		"web":       {alertSourceJSON("web"), "network.alert", "high", true},
		"vuln":      {alertSourceJSON("vuln"), "vulnerability.detection", "high", false},
		"office365": {alertSourceJSON("office365"), "cloud.audit", "low", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var source alertSource
			if err := json.Unmarshal([]byte(tc.source), &source); err != nil {
				t.Fatal(err)
			}
			mapped, err := mapHit("wazuh-alerts-4.x-sample-security", "doc", source, time.Unix(10, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			if mapped.Event.Type != tc.wantType || mapped.Event.Severity != tc.wantSev {
				t.Fatalf("type/sev = %s/%s", mapped.Event.Type, mapped.Event.Severity)
			}
			if _, ok := mapped.Event.Attributes["correlation_event_id"]; ok {
				t.Fatal("correlation_event_id must not be emitted")
			}
			corrType, _ := mapped.Event.Attributes["correlation_type"].(string)
			if tc.freq {
				if corrType != "wazuh_frequency" {
					t.Fatalf("correlation_type = %q", corrType)
				}
				if mapped.Event.Attributes["correlation_name"] == nil {
					t.Fatal("expected correlation_name")
				}
			} else if corrType != "wazuh_rule" {
				t.Fatalf("correlation_type = %q", corrType)
			}
			if mapped.Event.Attributes["rule.id"] == nil || mapped.Event.Attributes["agent.name"] == nil {
				t.Fatalf("expected native attrs, got %v", mapped.Event.Attributes)
			}
			if mapped.Event.Attributes["rule_id"] != nil || mapped.Event.Attributes["agent_name"] != nil {
				t.Fatalf("legacy attr keys present: %v", mapped.Event.Attributes)
			}
			encoded := mustJSON(mapped)
			if strings.Contains(encoded, "full_log") || strings.Contains(encoded, "previous_output") {
				t.Fatal("sensitive fields leaked")
			}
			if len(mapped.Entities) == 0 {
				t.Fatal("expected entities")
			}
		})
	}
}

func TestPDQLFilterTranslation(t *testing.T) {
	query, err := filterToQuery(`rule.groups = "sshd" and rule.level >= 5`)
	if err != nil {
		t.Fatal(err)
	}
	raw := mustJSON(query)
	if !strings.Contains(raw, `"rule.groups"`) || !strings.Contains(raw, `"gte":5`) {
		t.Fatalf("%s", raw)
	}
	query, err = filterToQuery(`rule.description contains "ssh" or rule.level >= 10`)
	if err != nil {
		t.Fatal(err)
	}
	raw = mustJSON(query)
	if !strings.Contains(raw, "wildcard") || !strings.Contains(raw, "rule.level") {
		t.Fatalf("%s", raw)
	}
	if _, err := filterToQuery(`importance = "high"`); err == nil {
		t.Fatal("expected MaxPatrol alias rejection")
	}
	if _, err := filterToQuery(`event_src.host = "x"`); err == nil {
		t.Fatal("expected MaxPatrol alias rejection")
	}
	if _, err := filterToQuery(`object.process.cmdline contains "x"`); err == nil {
		t.Fatal("expected unsupported field")
	}
	if _, err := filterToQuery(`rule.groups = "sshd" | select(text)`); err == nil {
		t.Fatal("expected pipeline rejection")
	}
}

func TestAggregateCorrelationType(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "wazuh_frequency") {
			t.Fatalf("body=%s", raw)
		}
		_, _ = w.Write([]byte(`{
			"timed_out":false,"_shards":{"failed":0},"hits":{"total":{"value":0,"relation":"eq"},"hits":[]},
			"aggregations":{"groups":{"buckets":{"wazuh_frequency":{"doc_count":10},"wazuh_rule":{"doc_count":20}}}}
		}`))
	}))
	defer server.Close()
	provider := mustProvider(t, server.URL)
	page, err := provider.EventAggregation.AggregateEvents(context.Background(), capability.Access{}, capability.AggregateEventsRequest{
		TimeFrom: time.Unix(1, 0), TimeTo: time.Unix(2, 0), GroupBy: []string{"correlation_type"}, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Groups) != 2 {
		t.Fatalf("%#v", page.Groups)
	}
}

func TestErrorsDoNotLeakCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	provider := mustProvider(t, server.URL)
	_, err := provider.Events.SearchEvents(context.Background(), capability.Access{}, capability.SearchEventsRequest{
		TimeFrom: time.Unix(1, 0), TimeTo: time.Unix(2, 0), Limit: 10,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "admin") {
		t.Fatalf("leaked credentials: %v", err)
	}
}

func mustProvider(t *testing.T, baseURL string) registry.Provider {
	t.Helper()
	provider, err := NewProvider(ClientConfig{
		HTTP:         proxy.HTTPClientConfig{BaseURL: baseURL, Timeout: time.Second, SkipTLSVerify: true},
		Username:     "admin",
		Password:     "secret",
		IndexPattern: "wazuh-alerts-*",
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func searchPageJSON(index, id, ts string) string {
	sortVals, _ := json.Marshal([]any{ts, "1580123327.49031", index, id})
	return `{
		"timed_out":false,"_shards":{"failed":0},
		"hits":{"total":{"value":2,"relation":"eq"},"hits":[{
			"_index":"` + index + `","_id":"` + id + `","sort":` + string(sortVals) + `,
			"_source":` + alertSourceJSON("sshd") + `
		}]}
	}`
}

func alertSourceJSON(family string) string {
	switch family {
	case "syscheck":
		return `{
			"@timestamp":"2026-08-31T04:59:44.677Z","timestamp":"2026-08-31T04:59:44.677Z","id":"1",
			"agent":{"id":"003","name":"host-a","ip":"10.0.0.180"},
			"rule":{"id":"550","level":7,"description":"Integrity checksum changed.","groups":["wazuh","syscheck"]},
			"syscheck":{"path":"/etc/x","event":"modified","md5_after":"e84a809eadf6abfba37cf92380a98c54","sha1_after":"cbfdc768b9594079ed8dffe9381c4a4f0e40ea86","sha256_after":"373fea99bafa7dcc8c76e15fe7140e38f9961435b1ef32f0cfa847fbbd740754"}
		}`
	case "web":
		return `{
			"@timestamp":"2026-09-05T15:06:56.604Z","timestamp":"2026-09-05T15:06:56.604Z","id":"1",
			"agent":{"id":"003","name":"host-a","ip":"10.0.0.180"},
			"rule":{"id":"31151","level":10,"description":"Multiple web server 400 error codes from same source ip.","groups":["web","accesslog","web_scan","recon"],"frequency":14},
			"data":{"protocol":"GET","srcip":"187.80.4.18","id":"404","url":"/admin/index.php"},
			"decoder":{"name":"web-accesslog"}
		}`
	case "vuln":
		return `{
			"@timestamp":"2026-08-29T22:48:23.324Z","timestamp":"2026-08-29T22:48:23.324Z","id":"1",
			"agent":{"id":"003","name":"host-a","ip":"10.0.0.180"},
			"rule":{"id":"23505","level":10,"description":"CVE-2018-8769 affects elfutils","groups":["vulnerability-detector"]},
			"data":{"vulnerability":{"cve":"CVE-2018-8769","severity":"High","status":"Active","package":{"name":"elfutils","version":"0.170"},"cvss":{"cvss3":{"base_score":7.8}}}}
		}`
	case "office365":
		return `{
			"@timestamp":"2026-08-29T21:45:51.874Z","timestamp":"2026-08-29T21:45:51.874Z","id":"1",
			"agent":{"id":"000","name":"wazuh","ip":"1.2.3.4"},
			"rule":{"id":"91548","level":5,"description":"Office 365: Admin actions","groups":["office365"]},
			"data":{"office365":{"UserId":"brown@wazuh.com","ClientIP":"13.226.52.2","Operation":"Get-Report","Workload":"Security"}}
		}`
	default:
		return `{
			"@timestamp":"2026-09-05T05:00:04.299Z","timestamp":"2026-09-05T05:00:04.299Z","id":"1580123327.49031",
			"agent":{"id":"006","name":"Windows","ip":"207.45.34.78"},
			"rule":{"id":"5758","level":8,"description":"Maximum authentication attempts exceeded.","groups":["syslog","sshd","authentication_failed"],"frequency":4,
				"mitre":{"id":["T1110"],"tactic":["Credential Access"],"technique":["Brute Force"]}},
			"data":{"srcip":"134.87.21.47","dstuser":"SYSTEM","srcport":"26874"},
			"decoder":{"name":"sshd"},"location":"/var/log/secure"
		}`
	}
}

func asRequestError(err error, target **domain.RequestError) bool {
	for err != nil {
		if typed, ok := err.(*domain.RequestError); ok {
			*target = typed
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}