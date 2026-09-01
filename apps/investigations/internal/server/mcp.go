package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
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

type gatewayListSourcesArgs struct {
	Refresh *bool `json:"refresh,omitempty" jsonschema:"Bypass cached source statuses"`
}

type gatewaySearchEventsArgs struct {
	gatewaycontract.SearchEventsRequest
}
type gatewayAggregateEventsArgs struct {
	gatewaycontract.AggregateEventsRequest
}
type gatewayLookupEntityArgs struct {
	gatewaycontract.LookupEntityRequest
}
type gatewaySearchFindingsArgs struct {
	gatewaycontract.SearchFindingsRequest
}
type gatewaySearchSessionsArgs struct {
	gatewaycontract.SearchSessionsRequest
}
type gatewaySearchEndpointsArgs struct {
	gatewaycontract.SearchEndpointsRequest
}

type gatewayGetFindingArgs struct {
	Source         string                      `json:"source" jsonschema:"Gateway source code"`
	Kind           gatewaycontract.FindingKind `json:"kind" jsonschema:"siem_incident, siem_correlation, or nad_attack"`
	ExternalID     string                      `json:"external_id" jsonschema:"Finding identifier in the source"`
	SourceInstance *string                     `json:"source_instance,omitempty" jsonschema:"Provider instance such as a PT NAD store ID"`
	From           time.Time                   `json:"from" jsonschema:"Inclusive lookup window start"`
	To             time.Time                   `json:"to" jsonschema:"Inclusive lookup window end"`
}

type gatewayGetSessionArgs struct {
	Source         string    `json:"source" jsonschema:"Gateway source code"`
	ExternalID     string    `json:"external_id" jsonschema:"Session identifier in the source"`
	SourceInstance string    `json:"source_instance" jsonschema:"Provider instance such as a PT NAD store ID"`
	From           time.Time `json:"from" jsonschema:"Inclusive lookup window start"`
	To             time.Time `json:"to" jsonschema:"Inclusive lookup window end"`
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
	addGatewayTools(server, s)

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
		reflect.TypeFor[time.Time](): {Type: "string", Format: "date-time"},
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

func addGatewayTools(server *mcp.Server, s *Server) {
	mcp.AddTool(server, mcpTool[gatewayListSourcesArgs](
		"gateway_list_sources", "List project-allowed Gateway sources and their capabilities.",
	), gatewayHandler(s, func(ctx context.Context, args gatewayListSourcesArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.ListSources(ctx, scope.ProjectID, bearer, args.Refresh)
	}))
	mcp.AddTool(server, mcpTool[gatewaySearchEventsArgs](
		"gateway_search_events", "Search normalized events across project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaySearchEventsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchEvents(ctx, scope.ProjectID, bearer, args.SearchEventsRequest)
	}))
	mcp.AddTool(server, mcpTool[gatewayAggregateEventsArgs](
		"gateway_aggregate_events", "Group and count events using source-supported fields.",
	), gatewayHandler(s, func(ctx context.Context, args gatewayAggregateEventsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.AggregateEvents(ctx, scope.ProjectID, bearer, args.AggregateEventsRequest)
	}))
	mcp.AddTool(server, mcpTool[gatewayLookupEntityArgs](
		"gateway_lookup_entity", "Enrich an entity through project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewayLookupEntityArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.LookupEntity(ctx, scope.ProjectID, bearer, args.LookupEntityRequest)
	}))
	mcp.AddTool(server, mcpTool[gatewaySearchFindingsArgs](
		"gateway_search_findings", "Search source-native incidents, correlations, and attacks.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaySearchFindingsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchFindings(ctx, scope.ProjectID, bearer, args.SearchFindingsRequest)
	}))
	mcp.AddTool(server, mcpTool[gatewayGetFindingArgs](
		"gateway_get_finding", "Resolve one source-native finding and its current context.",
	), gatewayHandler(s, func(ctx context.Context, args gatewayGetFindingArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		if strings.TrimSpace(args.Source) == "" || strings.TrimSpace(args.ExternalID) == "" || !args.Kind.Valid() || args.From.IsZero() || !args.From.Before(args.To) {
			return nil, errors.New("invalid arguments: source, supported kind, external_id, and an increasing from/to window are required")
		}
		return s.gateway.GetFinding(ctx, scope.ProjectID, bearer, args.Source, args.Kind, args.ExternalID, args.SourceInstance, args.From, args.To)
	}))
	mcp.AddTool(server, mcpTool[gatewaySearchSessionsArgs](
		"gateway_search_sessions", "Search source-native network sessions.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaySearchSessionsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchSessions(ctx, scope.ProjectID, bearer, args.SearchSessionsRequest)
	}))
	mcp.AddTool(server, mcpTool[gatewayGetSessionArgs](
		"gateway_get_session", "Resolve one source-native network session.",
	), gatewayHandler(s, func(ctx context.Context, args gatewayGetSessionArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		if strings.TrimSpace(args.Source) == "" || strings.TrimSpace(args.ExternalID) == "" || strings.TrimSpace(args.SourceInstance) == "" || args.From.IsZero() || !args.From.Before(args.To) {
			return nil, errors.New("invalid arguments: source, external_id, source_instance, and an increasing from/to window are required")
		}
		return s.gateway.GetSession(ctx, scope.ProjectID, bearer, args.Source, args.ExternalID, args.SourceInstance, args.From, args.To)
	}))
	mcp.AddTool(server, mcpTool[gatewaySearchEndpointsArgs](
		"gateway_search_endpoints", "Search endpoint inventory through project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaySearchEndpointsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchEndpoints(ctx, scope.ProjectID, bearer, args.SearchEndpointsRequest)
	}))
}

func gatewayHandler[T any](s *Server, call func(context.Context, T, socctx.Scope, string) (json.RawMessage, error)) func(context.Context, *mcp.CallToolRequest, T) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args T) (*mcp.CallToolResult, any, error) {
		scope, err := s.scope(ctx)
		if err != nil {
			return mcpFailure(err)
		}
		bearer, ok := socctx.BearerFromContext(ctx)
		if !ok {
			return mcpFailure(httperr.ErrUnauthorized)
		}
		raw, err := call(ctx, args, scope, bearer)
		if err != nil {
			var upstream *gatewayclient.HTTPError
			if errors.As(err, &upstream) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: upstream.Message}}, IsError: true}, nil, nil
			}
			return mcpFailure(httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, "Gateway is unavailable"))
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return mcpFailure(err)
		}
		return nil, value, nil
	}
}
