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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

const (
	mcpGraphReadScope         = "investigation.graph.read"
	mcpEventsReadScope        = "investigation.events.read"
	mcpAgentResultsWriteScope = "investigation.agent_results.write"
)

type investigationArgs struct {
	InvestigationID string `json:"investigation_id"`
}

type listEventsArgs struct {
	InvestigationID string  `json:"investigation_id"`
	Limit           *int    `json:"limit,omitempty"`
	Cursor          *string `json:"cursor,omitempty"`
}

type addAgentResultsArgs struct {
	InvestigationID string                     `json:"investigation_id"`
	SomIssueIDs     []uuid.UUID                `json:"som_issue_ids"`
	Nodes           []mcpAgentNode             `json:"nodes"`
	Edges           []investigations.AgentEdge `json:"edges"`
}

type mcpAgentNode struct {
	Ref      string     `json:"ref"`
	EventID  *uuid.UUID `json:"event_id,omitempty"`
	EntityID *uuid.UUID `json:"entity_id,omitempty"`
	NodeID   *uuid.UUID `json:"node_id,omitempty"`
}

func (s *Server) MCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sb0rka-investigation",
		Version: "1.0.0",
	}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "get_investigation_graph",
		Description: "Read the investigation graph, including proposed agent edges.",
		InputSchema: investigationInputSchema(),
	}, s.getInvestigationGraphTool)
	server.AddTool(&mcp.Tool{
		Name:        "list_investigation_events",
		Description: "Read one page of the investigation timeline.",
		InputSchema: listEventsInputSchema(),
	}, s.listInvestigationEventsTool)
	server.AddTool(&mcp.Tool{
		Name:        "add_investigation_agent_results",
		Description: "Atomically add graph nodes for records already attached to the investigation and proposed evidence-backed edges.",
		InputSchema: addAgentResultsInputSchema(),
	}, s.addInvestigationAgentResultsTool)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		handler.ServeHTTP(w, r)
	})
}

func investigationInputSchema() map[string]any {
	id := map[string]any{"type": "string", "format": "uuid", "description": "Investigation UUID"}
	return map[string]any{
		"type": "object", "properties": map[string]any{"investigation_id": id},
		"required": []string{"investigation_id"}, "additionalProperties": false,
	}
}

func listEventsInputSchema() map[string]any {
	schema := investigationInputSchema()
	properties := schema["properties"].(map[string]any)
	properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 200}
	properties["cursor"] = map[string]any{"type": "string"}
	return schema
}

func addAgentResultsInputSchema() map[string]any {
	id := map[string]any{"type": "string", "format": "uuid"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"investigation_id": id,
			"som_issue_ids":    map[string]any{"type": "array", "minItems": 1, "items": id},
			"nodes": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"ref": map[string]any{"type": "string"}, "event_id": id, "entity_id": id, "node_id": id,
				}, "required": []string{"ref"}, "additionalProperties": false,
			}},
			"edges": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"source_ref": map[string]any{"type": "string"}, "target_ref": map[string]any{"type": "string"}, "relation_code": map[string]any{"type": "string"},
					"why": map[string]any{"type": "string"}, "confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					"evidence_event_refs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1},
				}, "required": []string{"source_ref", "target_ref", "relation_code", "why", "evidence_event_refs"}, "additionalProperties": false,
			}},
		},
		"required":             []string{"investigation_id", "som_issue_ids", "nodes", "edges"},
		"additionalProperties": false,
	}
}

func (s *Server) getInvestigationGraphTool(
	ctx context.Context,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var args investigationArgs
	if err := decodeMCPArguments(request.Params.Arguments, &args); err != nil {
		return mcpToolError(fmt.Errorf("invalid arguments: %w", err)), nil
	}
	id, err := requireMCPInvestigation(ctx, args.InvestigationID, mcpGraphReadScope)
	if err != nil {
		return mcpToolError(err), nil
	}
	response, err := s.GetGraph(ctx, graph.GetGraphRequestObject{
		InvestigationId: id,
		Params:          graph.GetGraphParams{},
	})
	if err != nil {
		return mcpToolError(err), nil
	}
	value, ok := response.(graph.GetGraph200JSONResponse)
	if !ok {
		return mcpToolError(errors.New("tool returned an unexpected response")), nil
	}
	return mcpToolResult(graph.Graph(value)), nil
}

func (s *Server) listInvestigationEventsTool(
	ctx context.Context,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var args listEventsArgs
	if err := decodeMCPArguments(request.Params.Arguments, &args); err != nil {
		return mcpToolError(fmt.Errorf("invalid arguments: %w", err)), nil
	}
	id, err := requireMCPInvestigation(ctx, args.InvestigationID, mcpEventsReadScope)
	if err != nil {
		return mcpToolError(err), nil
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
	response, err := s.ListEvents(ctx, events.ListEventsRequestObject{
		InvestigationId: id,
		Params: events.ListEventsParams{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		return mcpToolError(err), nil
	}
	value, ok := response.(events.ListEvents200JSONResponse)
	if !ok {
		return mcpToolError(errors.New("tool returned an unexpected response")), nil
	}
	return mcpToolResult(events.EventPage(value)), nil
}

func (s *Server) addInvestigationAgentResultsTool(
	ctx context.Context,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var args addAgentResultsArgs
	if err := decodeMCPArguments(request.Params.Arguments, &args); err != nil {
		return mcpToolError(fmt.Errorf("invalid arguments: %w", err)), nil
	}
	id, err := requireMCPInvestigation(ctx, args.InvestigationID, mcpAgentResultsWriteScope)
	if err != nil {
		return mcpToolError(err), nil
	}
	body := investigations.AddAgentResultsJSONRequestBody{
		Events:      []investigations.AgentEventSelection{},
		Entities:    []investigations.AgentEntitySelection{},
		Edges:       args.Edges,
		Nodes:       make([]investigations.AgentNode, 0, len(args.Nodes)),
		SomIssueIds: make([]openapi_types.UUID, 0, len(args.SomIssueIDs)),
	}
	for _, issueID := range args.SomIssueIDs {
		body.SomIssueIds = append(body.SomIssueIds, openapi_types.UUID(issueID))
	}
	for _, node := range args.Nodes {
		converted := investigations.AgentNode{Ref: node.Ref}
		if node.EventID != nil {
			value := openapi_types.UUID(*node.EventID)
			converted.EventId = &value
		}
		if node.EntityID != nil {
			value := openapi_types.UUID(*node.EntityID)
			converted.EntityId = &value
		}
		if node.NodeID != nil {
			value := openapi_types.UUID(*node.NodeID)
			converted.NodeId = &value
		}
		body.Nodes = append(body.Nodes, converted)
	}
	response, err := s.AddAgentResults(ctx, investigations.AddAgentResultsRequestObject{
		InvestigationId: id,
		Body:            &body,
	})
	if err != nil {
		return mcpToolError(err), nil
	}
	value, ok := response.(investigations.AddAgentResults201JSONResponse)
	if !ok {
		return mcpToolError(errors.New("tool returned an unexpected response")), nil
	}
	return mcpToolResult(investigations.ContextImportResult(value)), nil
}

func decodeMCPArguments(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func mcpToolResult(value any) *mcp.CallToolResult {
	encoded, err := json.Marshal(value)
	if err != nil {
		return mcpToolError(err)
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
		StructuredContent: value,
	}
}

func requireMCPInvestigation(ctx context.Context, rawID, requiredScope string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(rawID))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid arguments: investigation_id must be a non-zero UUID")
	}
	authorization, ok := socctx.AgentAuthorizationFromContext(ctx)
	if ok && authorization.InvestigationID != id.String() {
		return uuid.Nil, httperr.New(http.StatusForbidden, httperr.CodeForbidden,
			"agent token does not grant access to this investigation")
	}
	if ok && !authorization.HasScope(requiredScope) {
		return uuid.Nil, httperr.New(http.StatusForbidden, httperr.CodeForbidden,
			"agent token does not grant the required scope")
	}
	return id, nil
}

func mcpToolError(err error) *mcp.CallToolResult {
	message := "internal server error"
	var domain *httperr.Error
	if errors.As(err, &domain) {
		message = domain.Message
	} else if err != nil && strings.HasPrefix(err.Error(), "invalid arguments") {
		message = err.Error()
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}
