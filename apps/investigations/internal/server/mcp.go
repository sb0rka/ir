package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

const mcpProtocolVersion = "2025-06-18"

type mcpCapabilityContextKey struct{}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      any          `json:"id"`
	Result  any          `json:"result,omitempty"`
	Error   *mcpRPCError `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpToolResult struct {
	Content           []mcpContent `json:"content"`
	StructuredContent any          `json:"structuredContent,omitempty"`
	IsError           bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type investigationArgs struct {
	InvestigationID uuid.UUID `json:"investigation_id"`
}

type listEventsArgs struct {
	InvestigationID uuid.UUID `json:"investigation_id"`
	Limit           *int      `json:"limit,omitempty"`
	Cursor          *string   `json:"cursor,omitempty"`
}

type addAgentResultsArgs struct {
	InvestigationID uuid.UUID `json:"investigation_id"`
	investigations.AgentResultBatch
}

func (s *Server) MCPHandler() http.Handler { return http.HandlerFunc(s.serveMCP) }

func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	if token := r.Header.Get("X-Sb0rka-MCP-Token"); token != "" {
		capability, ok := s.consumeMCPToken(token)
		if !ok {
			http.Error(w, "invalid or expired MCP capability", http.StatusUnauthorized)
			return
		}
		ctx := socctx.WithScope(r.Context(), socctx.Scope{ProjectID: capability.ProjectID})
		ctx = context.WithValue(ctx, mcpCapabilityContextKey{}, capability)
		r = r.WithContext(ctx)
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != mcpProtocolVersion {
		s.writeMCPError(w, nil, -32600, "unsupported MCP protocol version", nil)
		return
	}
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
		s.writeMCPError(w, nil, -32600, "Accept must include application/json and text/event-stream", nil)
		return
	}

	var request mcpRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		s.writeMCPError(w, nil, -32600, "invalid request", nil)
		return
	}
	if len(request.ID) == 0 || string(request.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var id any
	if err := json.Unmarshal(request.ID, &id); err != nil {
		s.writeMCPError(w, nil, -32600, "invalid request id", nil)
		return
	}

	switch request.Method {
	case "initialize":
		s.writeMCPResult(w, id, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "sb0rka-investigation", "version": "1.0.0"},
		})
	case "ping":
		s.writeMCPResult(w, id, map[string]any{})
	case "tools/list":
		s.writeMCPResult(w, id, map[string]any{"tools": investigationMCPTools()})
	case "tools/call":
		var call mcpToolCall
		if err := decodeMCPParams(request.Params, &call); err != nil {
			s.writeMCPError(w, id, -32602, "invalid tool call", err.Error())
			return
		}
		result := s.callMCPTool(r, call)
		s.writeMCPResult(w, id, result)
	default:
		s.writeMCPError(w, id, -32601, "method not found", nil)
	}
}

func investigationMCPTools() []map[string]any {
	id := map[string]any{"type": "string", "format": "uuid", "description": "Investigation UUID"}
	return []map[string]any{
		{"name": "get_investigation_graph", "description": "Read the investigation graph, including proposed agent edges.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"investigation_id": id}, "required": []string{"investigation_id"}, "additionalProperties": false}},
		{"name": "list_investigation_events", "description": "Read one page of the investigation timeline.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"investigation_id": id, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 200}, "cursor": map[string]any{"type": "string"}}, "required": []string{"investigation_id"}, "additionalProperties": false}},
		{"name": "add_investigation_agent_results", "description": "Atomically add selected records, graph nodes, and proposed evidence-backed edges from an agent run.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"investigation_id": id,
			"som_issue_ids":    map[string]any{"type": "array", "minItems": 1, "items": id},
			"events":           map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"ref": map[string]any{"type": "string"}, "source_code": map[string]any{"type": "string"}, "source_event_id": map[string]any{"type": "string"}}, "required": []string{"ref", "source_code", "source_event_id"}}},
			"entities":         map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"ref": map[string]any{"type": "string"}, "source_code": map[string]any{"type": "string"}, "source_entity_id": map[string]any{"type": "string"}}, "required": []string{"ref", "source_code", "source_entity_id"}}},
			"nodes":            map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"ref": map[string]any{"type": "string"}, "event_ref": map[string]any{"type": "string"}, "entity_ref": map[string]any{"type": "string"}, "node_id": id}, "required": []string{"ref"}}},
			"edges":            map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"source_ref": map[string]any{"type": "string"}, "target_ref": map[string]any{"type": "string"}, "relation_code": map[string]any{"type": "string"}, "why": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "evidence_event_refs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}}, "required": []string{"source_ref", "target_ref", "relation_code", "why", "evidence_event_refs"}}},
		}, "required": []string{"investigation_id", "som_issue_ids", "events", "entities", "nodes", "edges"}, "additionalProperties": false}},
	}
}

func (s *Server) callMCPTool(r *http.Request, call mcpToolCall) mcpToolResult {
	var value any
	var err error
	switch call.Name {
	case "get_investigation_graph":
		var args investigationArgs
		if decodeErr := decodeMCPParams(call.Arguments, &args); decodeErr != nil || args.InvestigationID == uuid.Nil {
			return mcpToolError(fmt.Errorf("invalid arguments: investigation_id must be a non-zero UUID"))
		}
		if err := requireMCPInvestigation(r, args.InvestigationID); err != nil {
			return mcpToolError(err)
		}
		var response graph.GetGraphResponseObject
		response, err = s.GetGraph(r.Context(), graph.GetGraphRequestObject{InvestigationId: args.InvestigationID, Params: graph.GetGraphParams{}})
		if ok, typeOK := response.(graph.GetGraph200JSONResponse); typeOK {
			value = graph.Graph(ok)
		}
	case "list_investigation_events":
		var args listEventsArgs
		if decodeErr := decodeMCPParams(call.Arguments, &args); decodeErr != nil || args.InvestigationID == uuid.Nil {
			return mcpToolError(fmt.Errorf("invalid arguments: investigation_id must be a non-zero UUID"))
		}
		if err := requireMCPInvestigation(r, args.InvestigationID); err != nil {
			return mcpToolError(err)
		}
		var limit *events.Limit
		if args.Limit != nil {
			converted := events.Limit(*args.Limit)
			limit = &converted
		}
		var cursor *events.Cursor
		if args.Cursor != nil {
			converted := events.Cursor(*args.Cursor)
			cursor = &converted
		}
		var response events.ListEventsResponseObject
		response, err = s.ListEvents(r.Context(), events.ListEventsRequestObject{InvestigationId: args.InvestigationID, Params: events.ListEventsParams{Limit: limit, Cursor: cursor}})
		if ok, typeOK := response.(events.ListEvents200JSONResponse); typeOK {
			value = events.EventPage(ok)
		}
	case "add_investigation_agent_results":
		var args addAgentResultsArgs
		if decodeErr := decodeMCPParams(call.Arguments, &args); decodeErr != nil || args.InvestigationID == uuid.Nil {
			return mcpToolError(fmt.Errorf("invalid arguments: investigation_id and the complete agent result batch are required"))
		}
		if err := requireMCPInvestigation(r, args.InvestigationID); err != nil {
			return mcpToolError(err)
		}
		body := investigations.AddAgentResultsJSONRequestBody(args.AgentResultBatch)
		var response investigations.AddAgentResultsResponseObject
		response, err = s.AddAgentResults(r.Context(), investigations.AddAgentResultsRequestObject{InvestigationId: args.InvestigationID, Body: &body})
		if ok, typeOK := response.(investigations.AddAgentResults201JSONResponse); typeOK {
			value = investigations.ContextImportResult(ok)
		}
	default:
		return mcpToolError(fmt.Errorf("unknown tool %q", call.Name))
	}
	if err != nil {
		return mcpToolError(err)
	}
	if value == nil {
		return mcpToolError(errors.New("tool returned an unexpected response"))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return mcpToolError(err)
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: string(encoded)}}, StructuredContent: value}
}

func requireMCPInvestigation(r *http.Request, investigationID uuid.UUID) error {
	capability, ok := r.Context().Value(mcpCapabilityContextKey{}).(mcpCapability)
	if ok && capability.InvestigationID != investigationID.String() {
		return httperr.New(http.StatusForbidden, httperr.CodeForbidden,
			"MCP capability does not grant access to this investigation")
	}
	return nil
}

func decodeMCPParams(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return errors.New("params are required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func mcpToolError(err error) mcpToolResult {
	message := "internal server error"
	var domain *httperr.Error
	if errors.As(err, &domain) {
		message = domain.Message
	} else if err != nil && (strings.HasPrefix(err.Error(), "invalid arguments") || strings.HasPrefix(err.Error(), "unknown tool")) {
		message = err.Error()
	}
	return mcpToolResult{Content: []mcpContent{{Type: "text", Text: message}}, IsError: true}
}

func (s *Server) writeMCPResult(w http.ResponseWriter, id, result any) {
	s.writeMCP(w, http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeMCPError(w http.ResponseWriter, id any, code int, message string, data any) {
	s.writeMCP(w, http.StatusOK, mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: code, Message: message, Data: data}})
}

func (s *Server) writeMCP(w http.ResponseWriter, status int, response mcpResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil && s.log != nil {
		s.log.Error("mcp_response_encode_failed", "error", err)
	}
}
