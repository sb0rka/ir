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
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
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
	InvestigationID string                                `json:"investigation_id"`
	SomIssueIDs     []uuid.UUID                           `json:"som_issue_ids"`
	Events          []investigations.AgentEventSelection  `json:"events"`
	Entities        []investigations.AgentEntitySelection `json:"entities"`
	Nodes           []mcpAgentNode                        `json:"nodes"`
	Edges           []investigations.AgentEdge            `json:"edges"`
}

type mcpAgentNode struct {
	Ref       string     `json:"ref"`
	EventID   *uuid.UUID `json:"event_id,omitempty"`
	EntityID  *uuid.UUID `json:"entity_id,omitempty"`
	NodeID    *uuid.UUID `json:"node_id,omitempty"`
	EventRef  *string    `json:"event_ref,omitempty"`
	EntityRef *string    `json:"entity_ref,omitempty"`
}

type searchGatewayEventsArgs struct {
	InvestigationID string                     `json:"investigation_id"`
	Sources         *[]string                  `json:"sources,omitempty"`
	TimeRange       gatewayclient.TimeRange    `json:"time_range"`
	Entities        *[]gatewayclient.EntityRef `json:"entities,omitempty"`
	Filter          *string                    `json:"filter,omitempty"`
	Columns         *[]string                  `json:"columns,omitempty"`
	Sort            *[]gatewayclient.EventSort `json:"sort,omitempty"`
	GroupBy         *[]string                  `json:"group_by,omitempty"`
	GroupValues     *[]*string                 `json:"group_values,omitempty"`
	Limit           *int                       `json:"limit,omitempty"`
	Cursor          *string                    `json:"cursor,omitempty"`
}

type lookupGatewayEntityArgs struct {
	InvestigationID string                  `json:"investigation_id"`
	Sources         *[]string               `json:"sources,omitempty"`
	Entity          gatewayclient.EntityRef `json:"entity"`
	TimeRange       gatewayclient.TimeRange `json:"time_range"`
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
	server.AddTool(&mcp.Tool{
		Name:        "search_gateway_events",
		Description: "Search project-allowed Gateway event sources while staying bound to this investigation.",
		InputSchema: searchGatewayEventsInputSchema(),
	}, s.searchGatewayEventsTool)
	server.AddTool(&mcp.Tool{
		Name:        "lookup_gateway_entity",
		Description: "Enrich one entity through project-allowed Gateway sources while staying bound to this investigation.",
		InputSchema: lookupGatewayEntityInputSchema(),
	}, s.lookupGatewayEntityTool)

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
			"events": map[string]any{"type": "array", "maxItems": 500, "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"ref": map[string]any{"type": "string"}, "source_code": map[string]any{"type": "string"}, "source_event_id": map[string]any{"type": "string"},
				}, "required": []string{"ref", "source_code", "source_event_id"}, "additionalProperties": false,
			}},
			"entities": map[string]any{"type": "array", "maxItems": 2000, "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"ref": map[string]any{"type": "string"}, "source_code": map[string]any{"type": "string"}, "source_entity_id": map[string]any{"type": "string"},
				}, "required": []string{"ref", "source_code", "source_entity_id"}, "additionalProperties": false,
			}},
			"nodes": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"ref": map[string]any{"type": "string"}, "event_ref": map[string]any{"type": "string"}, "entity_ref": map[string]any{"type": "string"},
					"event_id": id, "entity_id": id, "node_id": id,
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
		"required":             []string{"investigation_id", "som_issue_ids", "events", "entities", "nodes", "edges"},
		"additionalProperties": false,
	}
}

func gatewayTimeRangeSchema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"from": map[string]any{"type": "string", "format": "date-time"},
			"to":   map[string]any{"type": "string", "format": "date-time"},
		}, "required": []string{"from", "to"}, "additionalProperties": false,
	}
}

func gatewayEntityRefSchema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"type": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"},
		}, "required": []string{"type", "value"}, "additionalProperties": false,
	}
}

func searchGatewayEventsInputSchema() map[string]any {
	id := map[string]any{"type": "string", "format": "uuid"}
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"investigation_id": id, "sources": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true},
			"time_range": gatewayTimeRangeSchema(), "entities": map[string]any{"type": "array", "maxItems": 100, "items": gatewayEntityRefSchema()},
			"filter": map[string]any{"type": "string", "maxLength": 4096}, "columns": map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{"type": "string"}},
			"sort": map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{
				"type": "object", "properties": map[string]any{"field": map[string]any{"type": "string"}, "direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}}},
				"required": []string{"field", "direction"}, "additionalProperties": false,
			}},
			"group_by":     map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "string"}},
			"group_values": map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": []string{"string", "null"}}},
			"limit":        map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "cursor": map[string]any{"type": "string"},
		}, "required": []string{"investigation_id", "time_range"}, "additionalProperties": false,
	}
}

func lookupGatewayEntityInputSchema() map[string]any {
	return map[string]any{
		"type": "object", "properties": map[string]any{
			"investigation_id": map[string]any{"type": "string", "format": "uuid"},
			"sources":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true},
			"entity":           gatewayEntityRefSchema(), "time_range": gatewayTimeRangeSchema(),
		}, "required": []string{"investigation_id", "entity", "time_range"}, "additionalProperties": false,
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
	id, err := requireMCPInvestigation(args.InvestigationID)
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
	id, err := requireMCPInvestigation(args.InvestigationID)
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
	id, err := requireMCPInvestigation(args.InvestigationID)
	if err != nil {
		return mcpToolError(err), nil
	}
	body := investigations.AddAgentResultsJSONRequestBody{
		Events:      args.Events,
		Entities:    args.Entities,
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
		converted.EventRef = node.EventRef
		converted.EntityRef = node.EntityRef
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

func (s *Server) searchGatewayEventsTool(
	ctx context.Context,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var args searchGatewayEventsArgs
	if err := decodeMCPArguments(request.Params.Arguments, &args); err != nil {
		return mcpToolError(fmt.Errorf("invalid arguments: %w", err)), nil
	}
	scope, err := s.requireMCPGatewayAccess(ctx, args.InvestigationID)
	if err != nil {
		return mcpToolError(err), nil
	}
	bearer, err := s.gatewayBearer(ctx)
	if err != nil {
		return mcpToolError(err), nil
	}
	response, err := s.gateway.SearchEvents(ctx, scope.ProjectID, bearer, gatewayclient.SearchEventsRequest{
		Sources: args.Sources, TimeRange: args.TimeRange, Entities: args.Entities, Filter: args.Filter,
		Columns: args.Columns, Sort: args.Sort, GroupBy: args.GroupBy, GroupValues: args.GroupValues,
		Limit: args.Limit, Cursor: args.Cursor,
	})
	if err != nil {
		return mcpToolError(gatewayError(err)), nil
	}
	return mcpToolResult(response), nil
}

func (s *Server) lookupGatewayEntityTool(
	ctx context.Context,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	var args lookupGatewayEntityArgs
	if err := decodeMCPArguments(request.Params.Arguments, &args); err != nil {
		return mcpToolError(fmt.Errorf("invalid arguments: %w", err)), nil
	}
	scope, err := s.requireMCPGatewayAccess(ctx, args.InvestigationID)
	if err != nil {
		return mcpToolError(err), nil
	}
	bearer, err := s.gatewayBearer(ctx)
	if err != nil {
		return mcpToolError(err), nil
	}
	response, err := s.gateway.LookupEntity(ctx, scope.ProjectID, bearer, gatewayclient.LookupEntityRequest{
		Sources: args.Sources, Entity: args.Entity, TimeRange: args.TimeRange,
	})
	if err != nil {
		return mcpToolError(gatewayError(err)), nil
	}
	return mcpToolResult(response), nil
}

func (s *Server) requireMCPGatewayAccess(ctx context.Context, rawInvestigationID string) (socctx.Scope, error) {
	id, err := requireMCPInvestigation(rawInvestigationID)
	if err != nil {
		return socctx.Scope{}, err
	}
	scope, err := s.scope(ctx)
	if err != nil {
		return socctx.Scope{}, err
	}
	if _, err := s.db.GetInvestigation(ctx, scope.ProjectID, id.String()); err != nil {
		return socctx.Scope{}, storeError(err)
	}
	return scope, nil
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

func requireMCPInvestigation(rawID string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(rawID))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid arguments: investigation_id must be a non-zero UUID")
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
