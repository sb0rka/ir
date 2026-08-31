package server

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

type investigationArgs struct {
	InvestigationID openapi_types.UUID `json:"investigation_id" jsonschema:"Investigation UUID"`
}

type listEventsArgs struct {
	InvestigationID openapi_types.UUID `json:"investigation_id" jsonschema:"Investigation UUID"`
	Limit           *events.Limit      `json:"limit,omitempty" jsonschema:"Page size from 1 to 200"`
	Cursor          *events.Cursor     `json:"cursor,omitempty" jsonschema:"Cursor returned by the previous page"`
}

type addAgentResultsArgs struct {
	InvestigationID openapi_types.UUID `json:"investigation_id" jsonschema:"Investigation UUID"`
	investigations.AgentResultBatch
}

func (s *Server) MCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sb0rka-investigation",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool[investigationArgs, any](server, mcpTool[investigationArgs](
		"get_investigation_graph",
		"Read the investigation graph, including proposed agent edges.",
	), s.getInvestigationGraphTool)
	mcp.AddTool[listEventsArgs, any](server, mcpTool[listEventsArgs](
		"list_investigation_events",
		"Read one page of the investigation timeline.",
	), s.listInvestigationEventsTool)
	mcp.AddTool[addAgentResultsArgs, any](server, mcpTool[addAgentResultsArgs](
		"add_investigation_agent_results",
		"Atomically add graph nodes and proposed evidence-backed edges.",
	), s.addInvestigationAgentResultsTool)

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

func mcpTool[T any](name, description string) *mcp.Tool {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[uuid.UUID](): {Type: "string", Format: "uuid"},
	}})
	if err != nil {
		panic(err)
	}
	return &mcp.Tool{Name: name, Description: description, InputSchema: schema}
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
	response, err := s.ListEvents(ctx, events.ListEventsRequestObject{
		InvestigationId: id,
		Params: events.ListEventsParams{
			Limit:  args.Limit,
			Cursor: args.Cursor,
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
	if len(args.SomIssueIds) == 0 || len(args.Events) > 500 || len(args.Entities) > 2000 {
		return mcpFailure(errors.New("invalid arguments: expected at least one SOM issue, at most 500 events and at most 2000 entities"))
	}
	for _, id := range args.SomIssueIds {
		if id == uuid.Nil {
			return mcpFailure(errors.New("invalid arguments: som_issue_ids must contain non-zero UUIDs"))
		}
	}

	body := investigations.AddAgentResultsJSONRequestBody(args.AgentResultBatch)
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

func requireMCPInvestigation(id openapi_types.UUID) (uuid.UUID, error) {
	if id == uuid.Nil {
		return uuid.Nil, errors.New("invalid arguments: investigation_id must be a non-zero UUID")
	}
	return id, nil
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
