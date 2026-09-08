package ptnad

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func fixture(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	return raw
}
func testNAD(t *testing.T, handler http.HandlerFunc) (*Client, *Provider) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewProvider(client, []int64{19, 23})
	if err != nil {
		t.Fatal(err)
	}
	return client, provider
}
func caseWindow() TimeRange {
	return TimeRange{From: time.Date(2023, 6, 8, 0, 0, 0, 0, time.UTC), To: time.Date(2023, 6, 11, 0, 0, 0, 0, time.UTC)}
}
func caseRef(kind, id string) domain.SourceObjectRef {
	w := caseWindow()
	return domain.SourceObjectRef{SourceCode: SourceCode, SourceInstance: "23", RecordType: kind, ExternalID: id, TimeRange: domain.TimeRange{From: w.From, To: w.To}}
}

func TestCaseFiltersBeforeVendorLimit(t *testing.T) {
	filters := []string{
		`alert.msg ~ "*PSEXEC*" OR smb.rqs.create.filename ~ "*PSEXESVC*"`,
		`host.port == 2222 && src.ip == 192.168.25.202 && dst.ip == 192.168.214.203 && alert.pr == 2`,
		`files.filename == "(ChromeUpdate.exe)"`,
		`(app_proto == "imap" && host.ip == 192.168.25.190) || dst.ip == 192.168.214.203`,
	}
	calls := 0
	client, _ := testNAD(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		query := string(raw)
		if r.URL.Path != "/api/v2/bql" || !strings.Contains(query, "AND (") || !strings.HasSuffix(query, "LIMIT 1\n") {
			t.Errorf("filter/limit missing: %s", query)
		}
		switch calls {
		case 1:
			if !strings.Contains(query, `FROM 'alert' WHERE 'msg' ~ '*PSEXEC*'`) || !strings.Contains(query, `FROM 'smb' WHERE 'rqs.create.filename' ~ '*PSEXESVC*'`) {
				t.Errorf("nested filter: %s", query)
			}
		case 2:
			if strings.Index(query, `'pr' == 2`) > strings.LastIndex(query, "LIMIT") {
				t.Error("filter applied after limit")
			}
		}
		io.WriteString(w, `{"total":0,"result":[]}`)
	})
	window := caseWindow()
	for i, filter := range filters {
		request := SearchRequest{StoreID: 23, From: window.From, To: window.To, Limit: 1, Filter: filter}
		var err error
		if i == 0 {
			_, err = client.SearchAttacks(context.Background(), request, Access{Cookie: "csrftoken=test; sessionid=test"})
		} else {
			_, err = client.SearchSessions(context.Background(), request, Access{Cookie: "csrftoken=test; sessionid=test"})
		}
		if err != nil {
			t.Fatalf("%s: %v", filter, err)
		}
	}
	if calls != len(filters) {
		t.Fatal(calls)
	}
}
func TestFilterRejectsUnsupportedAndInjection(t *testing.T) {
	for _, input := range []string{`src.ip == "garbage"`, `host.port == 65536`, `alert.pr == -1`, `unknown == "x"`, `app_proto != "imap"`, `files.filename ~ "x' OR 1"`, `app_proto == "imap"; SELECT * FROM flow`, `(host.port == 2222`, `host.port == 2 &&`, `app_proto == "x" OR`, `app_proto ~ "x\\y"`, strings.Repeat("(", 18) + `host.port == 2` + strings.Repeat(")", 18), strings.Repeat("x", 4097)} {
		if _, err := compileFilter(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	a, _ := compileFilter(`host.port == 1 OR host.port == 2 AND host.port == 3`)
	b, _ := compileFilter(`host.port == 1 || host.port == 2 && host.port == 3`)
	if a != b || a != "(('host.port' == 1) OR (('host.port' == 2) AND ('host.port' == 3)))" {
		t.Fatal(a, b)
	}
}
func TestRecordedMailAndCredentialFallback(t *testing.T) {
	var detail flowDetail
	fixture(t, "imap-session.json", &detail)
	session, err := mapFlowDetail(detail, 23, caseWindow(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	events, entities, relations := decomposeSession(session)
	var mail *domain.Event
	for i := range events {
		if events[i].Type == "network.mail" {
			mail = &events[i]
		}
	}
	if mail == nil || mail.Title != "Очень срочное обновление" || mail.Attributes["from"] != "attacker@mail.ru" || mail.Attributes["parent_session_id"] != detail.ID {
		t.Fatalf("mail: %+v", mail)
	}
	for _, value := range []struct{ kind, value string }{{"email", "attacker@mail.ru"}, {"email", "victim@ptpilot.local"}, {"account", "victim@ptpilot.local"}} {
		if !slices.ContainsFunc(entities, func(e domain.Entity) bool { return e.Type == value.kind && e.Value == value.value }) {
			t.Errorf("missing %+v in %+v", value, entities)
		}
	}
	for _, r := range relations {
		if r.Type == "attached_to" {
			t.Fatal("unproven attachment relation")
		}
	}
	detail.Credentials = []credentialDTO{{Login: "preferred", User: "fallback"}, {User: "fallback"}}
	session, err = mapFlowDetail(detail, 23, caseWindow(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if session.Authentication[0].Account != "preferred" || session.Authentication[1].Account != "fallback" {
		t.Fatal(session.Authentication)
	}
}
func TestRecordedPayloadExactAndIdentity(t *testing.T) {
	var detail alertDetail
	fixture(t, "shell-alert.json", &detail)
	raw, _ := json.Marshal(detail)
	badParent := false
	_, provider := testNAD(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/alert/"+detail.ID) || r.URL.Query().Get("source") != "23" {
			t.Error(r.URL)
		}
		if badParent {
			wrong := detail
			wrong.Parent = "other-session"
			json.NewEncoder(w).Encode(wrong)
			return
		}
		w.Write(raw)
	})
	handle := capability.EvidenceHandle{Reference: domain.EvidenceReference{Kind: "payload", Ref: caseRef(AttackRecordType, detail.ID)}, ParentID: detail.Parent}
	reader, err := provider.OpenEvidence(context.Background(), capability.Access{Cookie: "csrftoken=test"}, handle)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	want, err := base64.StdEncoding.DecodeString(detail.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) || !bytes.Contains(got, []byte("Microsoft Windows")) {
		t.Fatal("shell banner bytes changed")
	}
	mapped, err := mapAttackDetail(detail, 23, caseWindow(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	finding := canonicalFinding(mapped)
	encoded, _ := json.Marshal(finding)
	if len(finding.Evidence) != 1 || bytes.Contains(encoded, []byte(detail.Payload)) {
		t.Fatal("missing evidence ref or payload leaked into JSON")
	}
	badParent = true
	if _, err = provider.OpenEvidence(context.Background(), capability.Access{Cookie: "csrftoken=test"}, handle); err == nil {
		t.Fatal("accepted substituted parent")
	}
	found := Attack{ParentSession: &SourceRef{ExternalID: detail.Parent}, OccurredAt: time.Now()}
	enriched := provider.client.enrichAttack(context.Background(), found, AttackRef{StoreID: 23, ExternalID: detail.ID, TimeRange: caseWindow()}, Access{Cookie: "csrftoken=test"})
	if len(enriched.ContextErrors) == 0 || !enriched.OccurredAt.Equal(found.OccurredAt) {
		t.Fatal("partial root not preserved")
	}
}
func TestFileExportLifecycleAndReferenceValidation(t *testing.T) {
	var detail flowDetail
	raw := fixture(t, "file-session.json", &detail)
	taskID := "11111111-2222-4333-8444-555555555555"
	posts := 0
	content := []byte{'P', 'K', 0, 255, 1, 2, 3}
	_, provider := testNAD(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/flow/" + detail.ID:
			w.Write(raw)
		case "/api/v2/sources/getfile":
			posts++
			var body struct {
				IDs    []string `json:"id"`
				MD5    []string `json:"md5"`
				Source []string `json:"source"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || len(body.IDs) != 1 || body.IDs[0] != detail.ID || len(body.MD5) != 1 || body.MD5[0] != detail.Files[0].MD5 || len(body.Source) != 1 || body.Source[0] != "23" {
				t.Errorf("extraction request: %+v", body)
			}
			w.WriteHeader(202)
			io.WriteString(w, `{"id":"`+taskID+`","state":"PENDING"}`)
		case "/api/v2/tasks/" + taskID:
			io.WriteString(w, `{"id":"`+taskID+`","state":"SUCCESS","result":{"url":"/api/v2/download/`+taskID+`.zip","total_files":1,"extracted_files":1,"errors":[]}}`)
		case "/api/v2/download/" + taskID + ".zip":
			w.Write(content)
		default:
			t.Error(r.URL)
			http.NotFound(w, r)
		}
	})
	access := capability.Access{Cookie: "csrftoken=test"}
	ref := domain.EvidenceReference{Kind: "file", Ref: caseRef(SessionRecordType, detail.ID), ObjectID: detail.Files[0].ID}
	handle, err := provider.StartEvidence(context.Background(), access, ref)
	if err != nil || handle.State != "pending" {
		t.Fatal(handle, err)
	}
	handle, err = provider.PollEvidence(context.Background(), access, handle)
	if err != nil || handle.State != "ready" {
		t.Fatal(handle, err)
	}
	reader, err := provider.OpenEvidence(context.Background(), access, handle)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	reader.Close()
	if !bytes.Equal(got, content) {
		t.Fatal(got)
	}
	ref.ObjectID = "foreign-file"
	if _, err = provider.StartEvidence(context.Background(), access, ref); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	ref.Ref.SourceInstance = "99"
	if _, err = provider.StartEvidence(context.Background(), access, ref); err == nil {
		t.Fatal("accepted foreign source instance")
	}
	if posts != 1 {
		t.Fatalf("unexpected vendor tasks: %d", posts)
	}
	ref = domain.EvidenceReference{Kind: "pcap", Ref: caseRef(SessionRecordType, detail.ID)}
	if _, err = provider.StartEvidence(context.Background(), access, ref); !errors.Is(err, domain.ErrUnsupportedCapability) {
		t.Fatal(err)
	}
}
func TestTaskPartialFailureAndHostileLocation(t *testing.T) {
	id := "11111111-2222-4333-8444-555555555555"
	base := capability.EvidenceHandle{TaskID: id}
	for _, tc := range []struct {
		state            string
		total, extracted int
		want             string
	}{{"PENDING", 0, 0, "pending"}, {"SUCCESS", 2, 1, "partial"}, {"SUCCESS", 1, 0, "failed"}, {"FAILURE", 0, 0, "failed"}, {"SUCCESS", 1, 1, "ready"}} {
		task := exportTask{ID: id, State: tc.state}
		task.Result.URL = "/api/v2/download/" + id + ".zip"
		task.Result.Total = tc.total
		task.Result.Extracted = tc.extracted
		got, err := updateExportTask(base, task)
		if err != nil || got.State != tc.want {
			t.Fatal(got, err)
		}
		if tc.want == "partial" || tc.want == "failed" {
			if got.Error == "" {
				t.Fatal("missing explanation")
			}
		}
	}
	for _, url := range []string{"https://other.example/steal", "//other.example/file", "/api/v2/download/other.zip"} {
		task := exportTask{ID: id, State: "SUCCESS"}
		task.Result.URL = url
		if _, err := updateExportTask(base, task); err == nil {
			t.Fatal("accepted", url)
		}
	}
	if _, err := updateExportTask(base, exportTask{ID: "other", State: "PENDING"}); err == nil {
		t.Fatal("accepted substituted task")
	}
}

func TestResolveFindingKeepsDedicatedDetailAndPartialRoot(t *testing.T) {
	var detail alertDetail
	fixture(t, "shell-alert.json", &detail)
	search := fixture(t, "shell-search.json", nil)
	detail.MalwareFamily = []string{"test-family"}
	failDetail := false
	_, provider := testNAD(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/bql":
			w.Write(search)
		case "/api/v2/flow/" + detail.Parent + "/alert/" + detail.ID:
			if failDetail {
				http.Error(w, "unavailable", 503)
				return
			}
			json.NewEncoder(w).Encode(detail)
		case "/api/v2/flow/" + detail.Parent:
			embedded := detail
			embedded.Payload = ""
			embedded.MalwareFamily = nil
			json.NewEncoder(w).Encode(flowDetail{ID: detail.Parent, Start: detail.Timestamp, End: detail.Timestamp, Alerts: []alertDetail{embedded}})
		default:
			t.Error(r.URL)
			http.NotFound(w, r)
		}
	})
	for _, fail := range []bool{false, true} {
		failDetail = fail
		page, err := provider.ResolveFinding(context.Background(), capability.Access{Cookie: "csrftoken=test"}, caseRef(AttackRecordType, detail.ID), true)
		if err != nil || len(page.Findings) != 1 {
			t.Fatal(page, err)
		}
		finding := page.Findings[0]
		if !fail && (len(finding.Evidence) != 1 || finding.NADAttack == nil || finding.NADAttack.PayloadAvailable == nil || !*finding.NADAttack.PayloadAvailable || len(finding.NADAttack.MalwareFamily) != 1) {
			t.Fatalf("dedicated detail overwritten: %+v", finding)
		}
		if fail && !slices.ContainsFunc(page.Resolutions, func(r domain.ObjectResolution) bool {
			return r.Ref.ExternalID == detail.ID && r.Status == "partial" && len(r.Errors) > 0
		}) {
			t.Fatal("root enrichment failure not marked partial", page.Resolutions)
		}
	}
}
