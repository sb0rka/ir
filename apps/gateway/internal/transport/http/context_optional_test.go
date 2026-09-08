package httptransport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/api"
)

func TestRootOnlyContextStillRequiresProjectAndSourceAccess(t *testing.T) {
	server := &Server{projectSources: map[string]map[string]bool{"aabbccddee": {}}}
	handler := projectScope(server)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.ResolveContext(w, r, api.ResolveContextParams{})
	}))
	for _, tc := range []struct {
		project string
		status  int
	}{
		{"", http.StatusBadRequest},
		{"1122334455", http.StatusForbidden},
		{"aabbccddee", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/context/resolve", strings.NewReader(`{"expand_findings":false,"events":[{"source_code":"pt-maxpatrol-siem","source_event_id":"event-1"}]}`))
		request.Header.Set("X-Project-ID", tc.project)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("project %q: status=%d body=%s", tc.project, response.Code, response.Body.String())
		}
	}
}

func TestResolveContextRequestPreservesFalseAndExplicitEvents(t *testing.T) {
	var body api.ResolveContextRequest
	if err := json.Unmarshal([]byte(`{"expand_findings":false,"events":[{"source_code":"pt-maxpatrol-siem","source_event_id":"event-1"}]}`), &body); err != nil {
		t.Fatal(err)
	}
	request, err := (&Server{}).resolveContextRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if request.ExpandFindings == nil || *request.ExpandFindings || len(request.Events) != 1 || request.Events[0].SourceEventID != "event-1" {
		t.Fatalf("unexpected request: %+v", request)
	}
	for _, payload := range []string{`{"expand_findings":false}`, `{"expand_findings":false,"events":[{"source_code":"","source_event_id":"event-1"}]}`} {
		var invalid api.ResolveContextRequest
		if err := json.Unmarshal([]byte(payload), &invalid); err != nil {
			t.Fatal(err)
		}
		if _, err := (&Server{}).resolveContextRequest(invalid); err == nil {
			t.Fatalf("accepted invalid selection: %s", payload)
		}
	}
}
