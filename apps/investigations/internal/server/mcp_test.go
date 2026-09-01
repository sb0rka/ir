package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

type mcpRecordingDB struct {
	store.Database
	request model.ImportRequest
}

func (db *mcpRecordingDB) GetInvestigation(_ context.Context, projectID, investigationID string) (model.Investigation, error) {
	return model.Investigation{ID: investigationID, ProjectID: projectID}, nil
}

func (db *mcpRecordingDB) ImportContext(_ context.Context, request model.ImportRequest) (model.ImportStats, error) {
	db.request = request
	return model.ImportStats{Nodes: len(request.Nodes), Edges: len(request.Edges)}, nil
}

func TestMCPInitializeAndListTools(t *testing.T) {
	t.Parallel()
	handler := (&Server{}).MCPHandler()

	initialize := mcpPost(t, handler, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if initialize.Code != http.StatusOK || !strings.Contains(initialize.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize: status=%d body=%s", initialize.Code, initialize.Body.String())
	}

	listed := mcpPost(t, handler, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	for _, name := range []string{
		"get_investigation_graph", "list_investigation_events", "add_investigation_agent_results",
		"gateway_list_sources", "gateway_search_events", "gateway_aggregate_events",
		"gateway_lookup_entity", "gateway_search_findings", "gateway_get_finding",
		"gateway_search_sessions", "gateway_get_session", "gateway_search_endpoints",
	} {
		if !strings.Contains(listed.Body.String(), name) {
			t.Fatalf("tools/list missing %s: %s", name, listed.Body.String())
		}
	}
	for _, field := range []string{"event_id", "entity_id", "node_id", "event_ref", "entity_ref", "source_event_id", "source_entity_id"} {
		if !strings.Contains(listed.Body.String(), field) {
			t.Fatalf("tools/list schema missing %s: %s", field, listed.Body.String())
		}
	}
	if !strings.Contains(listed.Body.String(), `"format":"uuid"`) || strings.Contains(listed.Body.String(), `"minItems":16`) {
		t.Fatalf("tools/list must expose UUIDs as strings: %s", listed.Body.String())
	}
	for _, field := range []string{"time_range", "source_code", "record_type", "external_id"} {
		if !strings.Contains(listed.Body.String(), field) {
			t.Fatalf("tools/list Gateway schema missing generated field %s: %s", field, listed.Body.String())
		}
	}
	if strings.Contains(listed.Body.String(), "X-Project-ID") || strings.Contains(listed.Body.String(), "XProjectID") {
		t.Fatalf("tools/list must not expose server-owned project scope: %s", listed.Body.String())
	}
}

func TestMCPGatewayToolsForwardProjectAndBearer(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-access-jwt" || r.Header.Get("X-Project-ID") != "abcdef1234" {
			t.Fatalf("Gateway auth headers: authorization=%q project=%q", r.Header.Get("Authorization"), r.Header.Get("X-Project-ID"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sources":
			if r.Method != http.MethodGet || r.URL.Query().Has("refresh") {
				t.Fatalf("list sources request: method=%s query=%s", r.Method, r.URL.RawQuery)
			}
			seen["sources"] = true
			_, _ = w.Write([]byte(`{"items":[{"code":"mock","name":"Mock","kind":"siem","mode":"proxy","status":"online","capabilities":["events"]}]}`))
		case "/api/v1/events/search":
			if r.Method != http.MethodPost {
				t.Fatalf("search events method: %s", r.Method)
			}
			var body struct {
				TimeRange map[string]string `json:"time_range"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TimeRange["from"] == "" || body.TimeRange["to"] == "" {
				t.Fatalf("search events body: %#v err=%v", body, err)
			}
			seen["events"] = true
			_, _ = w.Write([]byte(`{"events":[],"entities":[],"relations":[],"source_states":[],"source_errors":[]}`))
		default:
			t.Fatalf("unexpected Gateway path: %s", r.URL.Path)
		}
	}))
	defer gateway.Close()
	server := &Server{gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL})}

	for _, body := range []string{
		`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"gateway_list_sources","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"gateway_search_events","arguments":{"time_range":{"from":"2026-08-31T00:00:00Z","to":"2026-09-01T00:00:00Z"},"limit":2}}}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Content-Type", "application/json")
		ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
		ctx = socctx.WithBearer(ctx, "user-access-jwt")
		recorder := httptest.NewRecorder()
		server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"isError":true`) {
			t.Fatalf("Gateway MCP call: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
	if !seen["sources"] || !seen["events"] {
		t.Fatalf("Gateway calls not observed: %#v", seen)
	}
}

func TestMCPGatewayValidationError(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"gateway_get_session","arguments":{"source_code":"pt-nad","source_instance":"19","record_type":"siem_incident","external_id":"session-1","time_range":{"from":"2026-08-31T00:00:00Z","to":"2026-09-01T00:00:00Z"}}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	ctx = socctx.WithBearer(ctx, "user-access-jwt")
	recorder := httptest.NewRecorder()
	(&Server{}).MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "invalid arguments") || strings.Contains(recorder.Body.String(), "Gateway is unavailable") {
		t.Fatalf("Gateway validation error: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMCPNotificationAndToolError(t *testing.T) {
	t.Parallel()
	handler := (&Server{}).MCPHandler()

	notification := mcpPost(t, handler, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if notification.Code != http.StatusAccepted || notification.Body.Len() != 0 {
		t.Fatalf("notification: status=%d body=%s", notification.Code, notification.Body.String())
	}

	called := mcpPost(t, handler, `{"jsonrpc":"2.0","id":"x","method":"tools/call","params":{"name":"get_investigation_graph","arguments":{"investigation_id":"00000000-0000-0000-0000-000000000000"}}}`)
	var response struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(mcpResponseJSON(t, called), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Result.IsError {
		t.Fatalf("expected tool error: %s", called.Body.String())
	}
}

func mcpResponseJSON(t *testing.T, response *httptest.ResponseRecorder) []byte {
	t.Helper()
	body := response.Body.Bytes()
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream") {
		return body
	}
	for _, line := range strings.Split(string(body), "\n") {
		if payload, ok := strings.CutPrefix(line, "data: "); ok {
			return []byte(payload)
		}
	}
	t.Fatalf("SSE response does not contain a data event: %s", body)
	return nil
}

func TestMCPAgentResultsUsesLocalIDsWithoutGateway(t *testing.T) {
	t.Parallel()
	db := &mcpRecordingDB{}
	server := &Server{db: db, gateway: nil}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"add_investigation_agent_results","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"events":[],"entities":[],"nodes":[{"ref":"event","event_id":"33333333-3333-3333-3333-333333333333"},{"ref":"entity","entity_id":"44444444-4444-4444-4444-444444444444"}],"edges":[{"source_ref":"event","target_ref":"entity","relation_code":"mentions","why":"event evidence","evidence_event_refs":["event"]}]}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"isError":true`) {
		t.Fatalf("agent results: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(db.request.Selection.Events) != 0 || len(db.request.Selection.Entities) != 0 {
		t.Fatalf("MCP created a Gateway selection: %#v", db.request.Selection)
	}
	if len(db.request.Nodes) != 2 || db.request.Nodes[0].EventID == nil || db.request.Nodes[1].EntityID == nil {
		t.Fatalf("local node locators were not preserved: %#v", db.request.Nodes)
	}
}

func TestMCPAgentResultsImportsGatewaySelections(t *testing.T) {
	t.Parallel()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/context/resolve" || r.Header.Get("Authorization") != "Bearer user-access-jwt" {
			t.Fatalf("unexpected Gateway resolve request: %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"findings":[],"sessions":[],"relations":[],"resolutions":[],"source_errors":[],
			"events":[{"source_code":"mock","source_event_id":"event-1","type":"network","title":"Suspicious connection","severity":"high","occurred_at":"2026-08-31T10:00:00Z","entities":[{"type":"ip","value":"192.0.2.10","roles":["target"]}],"attributes":{},"fetched_at":"2026-08-31T10:01:00Z"}],
			"entities":[{"type":"ip","value":"192.0.2.10","attributes":{},"sources":[{"source_code":"mock","source_entity_id":"entity-1","fetched_at":"2026-08-31T10:01:00Z"}]}]
		}`))
	}))
	defer gateway.Close()
	db := &mcpRecordingDB{}
	server := &Server{
		db:      db,
		gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL}),
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"add_investigation_agent_results","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"events":[{"ref":"selected-event","source_code":"mock","source_event_id":"event-1"}],"entities":[{"ref":"selected-entity","source_code":"mock","source_entity_id":"entity-1"}],"nodes":[{"ref":"event-node","event_ref":"selected-event"},{"ref":"entity-node","entity_ref":"selected-entity"}],"edges":[{"source_ref":"event-node","target_ref":"entity-node","relation_code":"targets","why":"event names the target","evidence_event_refs":["event-node"]}]}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	ctx = socctx.WithBearer(ctx, "user-access-jwt")
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"isError":true`) {
		t.Fatalf("Gateway import: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(db.request.Selection.Events) != 1 || len(db.request.Selection.Entities) != 1 || len(db.request.Nodes) != 2 {
		t.Fatalf("Gateway selection was not preserved: %#v", db.request)
	}
	if db.request.Nodes[0].SnapshotEventID == nil || db.request.Nodes[1].SnapshotEntityID == nil {
		t.Fatalf("Gateway refs were not resolved into snapshot locators: %#v", db.request.Nodes)
	}
}

func mcpPost(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
