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
	"github.com/sb0rka/ir/packages/contract/reference"
)

type investigationArgs struct {
	InvestigationID openapi_types.UUID  `json:"investigation_id" jsonschema:"Investigation UUID"`
	HypothesisID    *openapi_types.UUID `json:"hypothesis_id,omitempty" jsonschema:"Optional hypothesis UUID; when set, read only its graph projection"`
}

type listEventsArgs struct {
	InvestigationID openapi_types.UUID `json:"investigation_id" jsonschema:"Investigation UUID"`
	Limit           *events.Limit      `json:"limit,omitempty" jsonschema:"Page size from 1 to 200"`
	Cursor          *events.Cursor     `json:"cursor,omitempty" jsonschema:"Cursor returned by the previous page"`
}

type addAgentResultsArgs struct {
	InvestigationID openapi_types.UUID  `json:"investigation_id" jsonschema:"Investigation UUID"`
	HypothesisID    *openapi_types.UUID `json:"hypothesis_id,omitempty" jsonschema:"Optional active hypothesis UUID; when set, add explicit results to its graph projection"`
	investigations.AgentResultBatch
}

func (s *Server) MCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sb0rka-investigation",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool[investigationArgs, any](server, mcpTool[investigationArgs](
		"get_investigation_graph",
		"Read the investigation graph or an optional hypothesis graph projection, including proposed agent edges.",
	), s.getInvestigationGraphTool)
	mcp.AddTool[listEventsArgs, any](server, mcpTool[listEventsArgs](
		"list_investigation_events",
		"Read one page of the investigation timeline.",
	), s.listInvestigationEventsTool)
	mcp.AddTool[addAgentResultsArgs, any](server, mcpTool[addAgentResultsArgs](
		"add_investigation_agent_results",
		"Atomically add graph nodes and proposed evidence-backed edges, optionally scoped to an active hypothesis.",
	), s.addInvestigationAgentResultsTool)
	mcp.AddTool[struct{}, any](server, mcpTool[struct{}](
		"get_investigation_reference",
		"Read IR entity and relation dictionaries. Check relation endpoint kinds and direction before submitting edges.",
	), s.getInvestigationReferenceTool)
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
	if err := requireMCPHypothesis(args.HypothesisID); err != nil {
		return mcpFailure(err)
	}
	if args.HypothesisID != nil {
		response, err := s.GetHypothesisGraph(ctx, graph.GetHypothesisGraphRequestObject{
			InvestigationId: id,
			HypothesisId:    *args.HypothesisID,
			Params:          graph.GetHypothesisGraphParams{},
		})
		if err != nil {
			return mcpFailure(err)
		}
		value, ok := response.(graph.GetHypothesisGraph200JSONResponse)
		if !ok {
			return mcpFailure(errors.New("tool returned an unexpected response"))
		}
		return nil, graph.HypothesisGraph(value), nil
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
	if err := requireMCPHypothesis(args.HypothesisID); err != nil {
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

	if args.HypothesisID != nil {
		response, err := s.AddHypothesisAgentResults(ctx, investigations.AddHypothesisAgentResultsRequestObject{
			InvestigationId: id,
			HypothesisId:    *args.HypothesisID,
			Params:          investigations.AddHypothesisAgentResultsParams{},
			Body:            &body,
		})
		if err != nil {
			return mcpFailure(err)
		}
		value, ok := response.(investigations.AddHypothesisAgentResults201JSONResponse)
		if !ok {
			return mcpFailure(errors.New("tool returned an unexpected response"))
		}
		return nil, investigations.ContextImportResult(value), nil
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

func (s *Server) getInvestigationReferenceTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, any, error) {
	response, err := s.GetReference(ctx, reference.GetReferenceRequestObject{})
	if err != nil {
		return mcpFailure(err)
	}
	value, ok := response.(reference.GetReference200JSONResponse)
	if !ok {
		return mcpFailure(errors.New("tool returned an unexpected response"))
	}
	return nil, reference.Reference(value), nil
}

func requireMCPInvestigation(id openapi_types.UUID) (uuid.UUID, error) {
	if id == uuid.Nil {
		return uuid.Nil, errors.New("invalid arguments: investigation_id must be a non-zero UUID")
	}
	return id, nil
}

func requireMCPHypothesis(id *openapi_types.UUID) error {
	if id != nil && *id == uuid.Nil {
		return errors.New("invalid arguments: hypothesis_id must be a non-zero UUID")
	}
	return nil
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
	mcp.AddTool(server, mcpTool[struct{}](
		"gateway_list_sources", "List project-allowed Gateway sources and their capabilities.",
	), gatewayHandler(s, func(ctx context.Context, _ struct{}, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.ListSources(ctx, scope.ProjectID, bearer)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SearchEventsRequest](
		"gateway_search_events", "Search normalized events across project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SearchEventsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchEvents(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.AggregateEventsRequest](
		"gateway_aggregate_events", "Group and count events using source-supported fields.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.AggregateEventsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.AggregateEvents(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.LookupEntityRequest](
		"gateway_lookup_entity", "Enrich an entity through project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.LookupEntityRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.LookupEntity(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.ResolveContextRequest](
		"gateway_resolve_context",
		"Resolve selected finding, session, event, or entity references into normalized context. Read-only: persist only explicitly selected events and entities through add_investigation_agent_results.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.ResolveContextRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		value, err := s.gateway.ResolveContext(ctx, scope.ProjectID, bearer, args)
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SearchFindingsRequest](
		"gateway_search_findings", "Search source-native incidents, correlations, and attacks.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SearchFindingsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchFindings(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SourceObjectRef](
		"gateway_get_finding", "Resolve one source-native finding and its current context.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SourceObjectRef, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		if strings.TrimSpace(args.SourceCode) == "" || strings.TrimSpace(args.ExternalId) == "" ||
			!gatewaycontract.FindingKind(args.RecordType).Valid() || args.TimeRange.From.IsZero() || !args.TimeRange.From.Before(args.TimeRange.To) {
			return nil, errors.New("invalid arguments: source_code, finding record_type, external_id, and an increasing time_range are required")
		}
		return s.gateway.GetFinding(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SearchSessionsRequest](
		"gateway_search_sessions", "Search source-native network sessions.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SearchSessionsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchSessions(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SourceObjectRef](
		"gateway_get_session", "Resolve one source-native network session.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SourceObjectRef, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		if strings.TrimSpace(args.SourceCode) == "" || strings.TrimSpace(args.ExternalId) == "" || args.RecordType != gatewaycontract.SourceObjectRefRecordTypeNadSession ||
			args.SourceInstance == nil || strings.TrimSpace(*args.SourceInstance) == "" || args.TimeRange.From.IsZero() || !args.TimeRange.From.Before(args.TimeRange.To) {
			return nil, errors.New("invalid arguments: source_code, nad_session record_type, external_id, source_instance, and an increasing time_range are required")
		}
		return s.gateway.GetSession(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.SearchEndpointsRequest](
		"gateway_search_endpoints", "Search endpoint inventory through project-allowed sources.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.SearchEndpointsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.SearchEndpoints(ctx, scope.ProjectID, bearer, args)
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
			if strings.HasPrefix(err.Error(), "invalid arguments") {
				return mcpFailure(err)
			}
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
