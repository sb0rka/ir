package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPInitializeAndListTools(t *testing.T) {
	t.Parallel()
	handler := (&Server{}).MCPHandler()

	initialize := mcpPost(t, handler, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if initialize.Code != http.StatusOK || !strings.Contains(initialize.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize: status=%d body=%s", initialize.Code, initialize.Body.String())
	}

	listed := mcpPost(t, handler, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	for _, name := range []string{"get_investigation_graph", "list_investigation_events", "add_investigation_agent_results"} {
		if !strings.Contains(listed.Body.String(), name) {
			t.Fatalf("tools/list missing %s: %s", name, listed.Body.String())
		}
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
		Result mcpToolResult `json:"result"`
	}
	if err := json.Unmarshal(called.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Result.IsError {
		t.Fatalf("expected tool error: %s", called.Body.String())
	}
}

func TestMCPCapabilityIsBoundToOneInvestigation(t *testing.T) {
	t.Parallel()
	server := &Server{mcpTokens: make(map[string]mcpCapability)}
	token, err := server.issueMCPToken("abcdef1234", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"get_investigation_graph","arguments":{"investigation_id":"22222222-2222-2222-2222-222222222222"}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Sb0rka-MCP-Token", token)
	recorder := httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "does not grant access") {
		t.Fatalf("cross-investigation call: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list","params":{}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("X-Sb0rka-MCP-Token", "invalid")
	recorder = httptest.NewRecorder()
	server.MCPHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid capability got %d, want 401", recorder.Code)
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
