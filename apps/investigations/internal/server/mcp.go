package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

type investigationArgs struct {
	InvestigationID string `json:"investigation_id" jsonschema:"Investigation UUID"`
}

type listEventsArgs struct {
	InvestigationID string  `json:"investigation_id" jsonschema:"Investigation UUID"`
	Limit           *int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 200"`
	Cursor          *string `json:"cursor,omitempty" jsonschema:"Cursor returned by the previous page"`
}

type addAgentResultsArgs struct {
	InvestigationID string                                `json:"investigation_id" jsonschema:"Investigation UUID"`
	SomIssueIDs     []string                              `json:"som_issue_ids" jsonschema:"At least one SOM issue UUID"`
	Events          []investigations.AgentEventSelection  `json:"events" jsonschema:"Gateway event selections, at most 500"`
	Entities        []investigations.AgentEntitySelection `json:"entities" jsonschema:"Gateway entity selections, at most 2000"`
	Nodes           []mcpAgentNode                        `json:"nodes"`
	Edges           []investigations.AgentEdge            `json:"edges"`
}

type mcpAgentNode struct {
	Ref       string  `json:"ref"`
	EventID   *string `json:"event_id,omitempty" jsonschema:"Attached event UUID"`
	EntityID  *string `json:"entity_id,omitempty" jsonschema:"Attached entity UUID"`
	NodeID    *string `json:"node_id,omitempty" jsonschema:"Existing graph node UUID"`
	EventRef  *string `json:"event_ref,omitempty" jsonschema:"Local ref of a Gateway event selection"`
	EntityRef *string `json:"entity_ref,omitempty" jsonschema:"Local ref of a Gateway entity selection"`
}

func (s *Server) MCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sb0rka-investigation",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool[investigationArgs, any](server, &mcp.Tool{
		Name:        "get_investigation_graph",
		Description: "Read the investigation graph, including proposed agent edges.",
	}, s.getInvestigationGraphTool)
	mcp.AddTool[listEventsArgs, any](server, &mcp.Tool{
		Name:        "list_investigation_events",
		Description: "Read one page of the investigation timeline.",
	}, s.listInvestigationEventsTool)
	mcp.AddTool[addAgentResultsArgs, any](server, &mcp.Tool{
		Name:        "add_investigation_agent_results",
		Description: "Atomically add graph nodes and proposed evidence-backed edges.",
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

func (s *Server) getInvestigationGraphTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args investigationArgs,
) (*mcp.CallToolResult, any, error) {
	id, err := requireMCPInvestigation(args.InvestigationID)
	if err != nil {
		return mcpFailure(err)
	}
	response, err := s.GetGraph(ctx, graph.GetGraphRequestObject{
		InvestigationId: id,
		Params:          graph.GetGraphParams{},
	})
	if err != nil {
		return mcpFailure(err)
	}
	value, ok := response.(graph.GetGraph200JSONResponse)
	if !ok {
		return mcpFailure(errors.New("tool returned an unexpected response"))
	}
	return nil, graph.Graph(value), nil
}

func (s *Server) listInvestigationEventsTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args listEventsArgs,
) (*mcp.CallToolResult, any, error) {
	id, err := requireMCPInvestigation(args.InvestigationID)
	if err != nil {
		return mcpFailure(err)
	}
	if args.Limit != nil && (*args.Limit < 1 || *args.Limit > 200) {
		return mcpFailure(errors.New("invalid arguments: limit must be between 1 and 200"))
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
		return mcpFailure(err)
	}
	value, ok := response.(events.ListEvents200JSONResponse)
	if !ok {
		return mcpFailure(errors.New("tool returned an unexpected response"))
	}
	return nil, events.EventPage(value), nil
}

func (s *Server) addInvestigationAgentResultsTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args addAgentResultsArgs,
) (*mcp.CallToolResult, any, error) {
	id, err := requireMCPInvestigation(args.InvestigationID)
	if err != nil {
		return mcpFailure(err)
	}
	if len(args.SomIssueIDs) == 0 || len(args.Events) > 500 || len(args.Entities) > 2000 {
		return mcpFailure(errors.New("invalid arguments: expected at least one SOM issue, at most 500 events and at most 2000 entities"))
	}

	body := investigations.AddAgentResultsJSONRequestBody{
		Events:      args.Events,
		Entities:    args.Entities,
		Edges:       args.Edges,
		Nodes:       make([]investigations.AgentNode, 0, len(args.Nodes)),
		SomIssueIds: make([]openapi_types.UUID, 0, len(args.SomIssueIDs)),
	}
	for _, raw := range args.SomIssueIDs {
		issueID, err := parseMCPUUID("som_issue_ids", raw)
		if err != nil {
			return mcpFailure(err)
		}
		body.SomIssueIds = append(body.SomIssueIds, openapi_types.UUID(issueID))
	}
	for _, node := range args.Nodes {
		converted, err := convertMCPAgentNode(node)
		if err != nil {
			return mcpFailure(err)
		}
		body.Nodes = append(body.Nodes, converted)
	}
	for _, edge := range args.Edges {
		if edge.Confidence != nil && (*edge.Confidence < 0 || *edge.Confidence > 1) {
			return mcpFailure(errors.New("invalid arguments: edge confidence must be between 0 and 1"))
		}
	}

	response, err := s.AddAgentResults(ctx, investigations.AddAgentResultsRequestObject{
		InvestigationId: id,
		Body:            &body,
	})
	if err != nil {
		return mcpFailure(err)
	}
	value, ok := response.(investigations.AddAgentResults201JSONResponse)
	if !ok {
		return mcpFailure(errors.New("tool returned an unexpected response"))
	}
	return nil, investigations.ContextImportResult(value), nil
}

func convertMCPAgentNode(node mcpAgentNode) (investigations.AgentNode, error) {
	locators := 0
	for _, value := range []*string{node.EventID, node.EntityID, node.NodeID, node.EventRef, node.EntityRef} {
		if value != nil {
			locators++
		}
	}
	if strings.TrimSpace(node.Ref) == "" || locators != 1 {
		return investigations.AgentNode{}, errors.New("invalid arguments: each node needs a ref and exactly one locator")
	}

	converted := investigations.AgentNode{Ref: node.Ref, EventRef: node.EventRef, EntityRef: node.EntityRef}
	for _, pair := range []struct {
		raw    *string
		target **openapi_types.UUID
	}{
		{node.EventID, &converted.EventId},
		{node.EntityID, &converted.EntityId},
		{node.NodeID, &converted.NodeId},
	} {
		if pair.raw != nil {
			value, err := parseMCPUUID("node locator", *pair.raw)
			if err != nil {
				return investigations.AgentNode{}, err
			}
			convertedValue := openapi_types.UUID(value)
			*pair.target = &convertedValue
		}
	}
	if node.EventRef != nil && strings.TrimSpace(*node.EventRef) == "" ||
		node.EntityRef != nil && strings.TrimSpace(*node.EntityRef) == "" {
		return investigations.AgentNode{}, errors.New("invalid arguments: node locator cannot be empty")
	}
	return converted, nil
}

func parseMCPUUID(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid arguments: %s must be a non-zero UUID", field)
	}
	return id, nil
}

func requireMCPInvestigation(rawID string) (uuid.UUID, error) {
	return parseMCPUUID("investigation_id", rawID)
}

func mcpFailure(err error) (*mcp.CallToolResult, any, error) {
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
	}, nil, nil
}
