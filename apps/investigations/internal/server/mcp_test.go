package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

type mcpRecordingDB struct {
	store.Database
	request      model.ImportRequest
	entityCard   model.EntityCard
	entityCardOK bool
	timelineFrom *time.Time
	timelineTo   *time.Time
}

func (db *mcpRecordingDB) GetInvestigation(_ context.Context, projectID, investigationID string) (model.Investigation, error) {
	return model.Investigation{ID: investigationID, ProjectID: projectID}, nil
}

func (db *mcpRecordingDB) ImportContext(_ context.Context, request model.ImportRequest) (model.ImportStats, error) {
	db.request = request
	return model.ImportStats{
		Events: len(request.Selection.Events), Entities: len(request.Selection.Entities),
		Nodes: len(request.Nodes), Edges: len(request.Edges),
	}, nil
}

func (db *mcpRecordingDB) Reference(context.Context) (model.Reference, error) {
	return model.Reference{RelationTypes: []model.RelationType{{
		Code: "mentions", Title: "Mentions", SourceKind: "event", TargetKind: "entity", Directed: true,
	}}}, nil
}

func (db *mcpRecordingDB) GetEntityCard(context.Context, string, string) (model.EntityCard, error) {
	if !db.entityCardOK {
		return model.EntityCard{}, store.ErrRecordNotFound
	}
	return db.entityCard, nil
}

func (db *mcpRecordingDB) InvestigationTimelineBounds(context.Context, string, string) (*time.Time, *time.Time, error) {
	return db.timelineFrom, db.timelineTo, nil
}

func (db *mcpRecordingDB) FindAttachedEntityBySource(context.Context, string, string, string, string) (string, error) {
	return "", store.ErrRecordNotFound
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
		"get_investigation_graph", "list_investigation_events", "add_investigation_agent_results", "import_entity_events", "get_investigation_reference",
		"gateway_list_sources", "gateway_search_events", "gateway_aggregate_events",
		"gateway_lookup_entity", "gateway_resolve_context", "gateway_search_findings", "gateway_get_finding",
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
	for _, hint := range []string{
		"Batch-local events[].ref",
		"Batch-local entities[].ref",
		"not a URN",
		"exactly one locator",
		"never IR entity_id",
		"pt-maxpatrol-siem, not NAD",
		"never an IR entity UUID",
		"import_entity_events",
		"single backslash",
	} {
		if !strings.Contains(listed.Body.String(), hint) {
			t.Fatalf("tools/list must describe agent/gateway identity rules (%q): %s", hint, listed.Body.String())
		}
	}
	if strings.Contains(listed.Body.String(), `Windows accounts need \\\\`) {
		t.Fatalf("tools/list must not encourage double-escaping backslashes: %s", listed.Body.String())
	}
	if count := strings.Count(listed.Body.String(), `"hypothesis_id"`); count != 3 {
		t.Fatalf("tools/list must expose optional hypothesis scope for graph, agent results, and import_entity_events: count=%d body=%s", count, listed.Body.String())
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
		case "/api/v1/context/resolve":
			if r.Method != http.MethodPost {
				t.Fatalf("resolve context method: %s", r.Method)
			}
			var body struct {
				Sessions []struct {
					ExternalID string `json:"external_id"`
				} `json:"sessions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Sessions) != 1 || body.Sessions[0].ExternalID != "session-1" {
				t.Fatalf("resolve context body: %#v err=%v", body, err)
			}
			seen["context"] = true
			_, _ = w.Write([]byte(`{"findings":[],"sessions":[],"events":[],"entities":[],"relations":[],"resolutions":[],"source_errors":[]}`))
		default:
			t.Fatalf("unexpected Gateway path: %s", r.URL.Path)
		}
	}))
	defer gateway.Close()
	server := &Server{db: &mcpRecordingDB{}, gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL})}

	for _, body := range []string{
		`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"gateway_list_sources","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"gateway_search_events","arguments":{"time_range":{"from":"2026-08-31T00:00:00Z","to":"2026-09-01T00:00:00Z"},"limit":2}}}`,
		`{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"gateway_resolve_context","arguments":{"sessions":[{"source_code":"pt-nad","source_instance":"19","record_type":"nad_session","external_id":"session-1","time_range":{"from":"2026-08-31T00:00:00Z","to":"2026-09-01T00:00:00Z"}}]}}}`,
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
	if !seen["sources"] || !seen["events"] || !seen["context"] {
		t.Fatalf("Gateway calls not observed: %#v", seen)
	}

	referenceRequest := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":23,"method":"tools/call","params":{"name":"get_investigation_reference","arguments":{}}}`))
	referenceRequest.Header.Set("Accept", "application/json, text/event-stream")
	referenceRequest.Header.Set("Content-Type", "application/json")
	referenceRecorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(referenceRecorder, referenceRequest.WithContext(socctx.WithScope(referenceRequest.Context(), socctx.Scope{ProjectID: "abcdef1234"})))
	if referenceRecorder.Code != http.StatusOK || !strings.Contains(referenceRecorder.Body.String(), `"relation_types"`) ||
		!strings.Contains(referenceRecorder.Body.String(), `"source_kind":"event"`) || !strings.Contains(referenceRecorder.Body.String(), `"target_kind":"entity"`) {
		t.Fatalf("reference MCP call: status=%d body=%s", referenceRecorder.Code, referenceRecorder.Body.String())
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
	db := &hypothesisFakeDB{hypothesis: model.Hypothesis{Status: "active"}}
	server := &Server{db: db, gateway: nil}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"add_investigation_agent_results","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","hypothesis_id":"55555555-5555-5555-5555-555555555555","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"events":[],"entities":[],"nodes":[{"ref":"event","event_id":"33333333-3333-3333-3333-333333333333"},{"ref":"entity","entity_id":"44444444-4444-4444-4444-444444444444"},{"ref":"existing","node_id":"66666666-6666-6666-6666-666666666666"}],"edges":[{"source_ref":"event","target_ref":"entity","relation_code":"mentions","why":"event evidence","evidence_event_refs":["event"]}]}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"isError":true`) {
		t.Fatalf("agent results: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(db.lastImport.Selection.Events) != 0 || len(db.lastImport.Selection.Entities) != 0 {
		t.Fatalf("MCP created a Gateway selection: %#v", db.lastImport.Selection)
	}
	if db.lastImport.HypothesisID == nil || *db.lastImport.HypothesisID != "55555555-5555-5555-5555-555555555555" || !db.lastImport.RequireActiveHypothesis {
		t.Fatalf("hypothesis scope was not preserved: %#v", db.lastImport)
	}
	if len(db.lastImport.Nodes) != 3 || db.lastImport.Nodes[0].EventID == nil || db.lastImport.Nodes[1].EntityID == nil || db.lastImport.Nodes[2].NodeID == nil {
		t.Fatalf("local node locators were not preserved: %#v", db.lastImport.Nodes)
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

func TestMCPRejectsUUIDSourceEntityID(t *testing.T) {
	t.Parallel()
	server := &Server{db: &mcpRecordingDB{}, gateway: nil}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":30,"method":"tools/call","params":{"name":"add_investigation_agent_results","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"events":[],"entities":[{"ref":"bad","source_code":"mock","source_entity_id":"b71336ed-25f7-42fa-840a-688ceb087c74"}],"nodes":[{"ref":"n","entity_ref":"bad"}],"edges":[]}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"isError":true`) ||
		!strings.Contains(body, "looks like an IR UUID") || !strings.Contains(body, "nodes[].entity_id") {
		t.Fatalf("expected UUID source_entity_id rejection: status=%d body=%s", recorder.Code, body)
	}
}

func TestMCPImportEntityEventsByEntityID(t *testing.T) {
	t.Parallel()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/events/search":
			_, _ = w.Write([]byte(`{
				"events":[{"source_code":"pt-maxpatrol-siem","source_event_id":"06b54c00-6c1b-11f1-8044-d00d762d3dd7","type":"auth","title":"Logon","severity":"medium","occurred_at":"2025-10-23T10:00:00Z","entities":[{"type":"account","value":"dkrylova\\administrator","roles":["actor"]}],"attributes":{"raw":true},"fetched_at":"2025-10-23T10:01:00Z"}],
				"entities":[{"type":"account","value":"dkrylova\\administrator","attributes":{},"sources":[{"source_code":"pt-maxpatrol-siem","source_entity_id":"account:dkrylova\\administrator","fetched_at":"2025-10-23T10:01:00Z"}]}],
				"relations":[],"source_states":[{"source":"pt-maxpatrol-siem","status":"ok"}],"source_errors":[]
			}`))
		case "/api/v1/context/resolve":
			_, _ = w.Write([]byte(`{
				"findings":[],"sessions":[],"relations":[],"resolutions":[],"source_errors":[],
				"events":[{"source_code":"pt-maxpatrol-siem","source_event_id":"06b54c00-6c1b-11f1-8044-d00d762d3dd7","type":"auth","title":"Logon","severity":"medium","occurred_at":"2025-10-23T10:00:00Z","entities":[{"type":"account","value":"dkrylova\\administrator","roles":["actor"]}],"attributes":{},"fetched_at":"2025-10-23T10:01:00Z"}],
				"entities":[]
			}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer gateway.Close()

	from := time.Date(2025, 10, 22, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 10, 24, 0, 0, 0, 0, time.UTC)
	db := &mcpRecordingDB{
		entityCardOK: true,
		entityCard: model.EntityCard{
			Entity: model.Entity{
				ID: "b71336ed-25f7-42fa-840a-688ceb087c74", TypeCode: "account",
				CanonicalKey: `dkrylova\administrator`,
				Sources: []model.EntitySource{{
					SourceCode: "pt-maxpatrol-siem", SourceEntityID: `account:dkrylova\administrator`,
				}},
			},
			Occurrences: []model.EntityOccurrence{{InvestigationID: "11111111-1111-1111-1111-111111111111"}},
		},
		timelineFrom: &from,
		timelineTo:   &to,
	}
	server := &Server{db: db, gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL})}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"import_entity_events","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"entity":{"entity_id":"b71336ed-25f7-42fa-840a-688ceb087c74"},"limit":10}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	ctx = socctx.WithBearer(ctx, "user-access-jwt")
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, `"isError":true`) {
		t.Fatalf("import_entity_events: status=%d body=%s", recorder.Code, body)
	}
	if !strings.Contains(body, `"events_found":1`) || !strings.Contains(body, `"events_imported":1`) {
		t.Fatalf("summary missing: %s", body)
	}
	if len(db.request.Nodes) < 2 || len(db.request.Edges) < 1 {
		t.Fatalf("expected entity+event nodes and role edge: %#v", db.request)
	}
	if db.request.Nodes[0].EntityID == nil || *db.request.Nodes[0].EntityID != "b71336ed-25f7-42fa-840a-688ceb087c74" {
		t.Fatalf("expected attached entity_id locator: %#v", db.request.Nodes[0])
	}
}

func TestMCPImportEntityEventsEmptySearch(t *testing.T) {
	t.Parallel()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"entities":[],"relations":[],"source_states":[],"source_errors":[]}`))
	}))
	defer gateway.Close()
	db := &mcpRecordingDB{entityCardOK: true, entityCard: model.EntityCard{
		Entity:      model.Entity{ID: "b71336ed-25f7-42fa-840a-688ceb087c74", TypeCode: "account", CanonicalKey: `dkrylova\administrator`},
		Occurrences: []model.EntityOccurrence{{InvestigationID: "11111111-1111-1111-1111-111111111111"}},
	}}
	server := &Server{db: db, gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL})}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":32,"method":"tools/call","params":{"name":"import_entity_events","arguments":{"investigation_id":"11111111-1111-1111-1111-111111111111","som_issue_ids":["22222222-2222-2222-2222-222222222222"],"entity":{"entity_id":"b71336ed-25f7-42fa-840a-688ceb087c74"},"time_range":{"from":"2025-10-22T00:00:00Z","to":"2025-10-24T00:00:00Z"}}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	ctx := socctx.WithScope(request.Context(), socctx.Scope{ProjectID: "abcdef1234"})
	ctx = socctx.WithBearer(ctx, "user-access-jwt")
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request.WithContext(ctx))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, `"isError":true`) {
		t.Fatalf("empty import: status=%d body=%s", recorder.Code, body)
	}
	if !strings.Contains(body, `"events_found":0`) || !strings.Contains(body, `"events_imported":0`) {
		t.Fatalf("expected zero summary: %s", body)
	}
	if len(db.request.Nodes) != 0 {
		t.Fatalf("empty search must not write: %#v", db.request)
	}
}

func TestNormalizeAccountBackslash(t *testing.T) {
	t.Parallel()
	if got := normalizeEntityValue("account", `dkrylova\\administrator`); got != `dkrylova\administrator` {
		t.Fatalf("normalizeEntityValue: %q", got)
	}
	if got := normalizeSourceEntityID(`account:dkrylova\\administrator`); got != `account:dkrylova\administrator` {
		t.Fatalf("normalizeSourceEntityID: %q", got)
	}
	if !looksLikeBareUUID("b71336ed-25f7-42fa-840a-688ceb087c74") {
		t.Fatal("expected bare UUID detection")
	}
	if looksLikeBareUUID(`account:dkrylova\administrator`) {
		t.Fatal("prefixed source id must not look like bare UUID")
	}
}
