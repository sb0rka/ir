package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/events"
	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
	"github.com/sb0rka/ir/packages/contract/reference"
)

type investigationArgs struct {
	Projection      string              `json:"projection,omitempty" jsonschema:"raw (default) or grouped; grouped returns lossless expansion restricted to this investigation or hypothesis, never the entire tree"`
	InvestigationID openapi_types.UUID  `json:"investigation_id" jsonschema:"Investigation UUID"`
	HypothesisID    *openapi_types.UUID `json:"hypothesis_id,omitempty" jsonschema:"Optional hypothesis UUID; when set, read only its graph projection"`
}

type listEventsArgs struct {
	InvestigationID openapi_types.UUID `json:"investigation_id" jsonschema:"Investigation UUID"`
	Limit           *events.Limit      `json:"limit,omitempty" jsonschema:"Page size from 1 to 200"`
	Cursor          *events.Cursor     `json:"cursor,omitempty" jsonschema:"Cursor returned by the previous page"`
}

// MCP-facing AgentResultBatch fields carry descriptions that generated OpenAPI
// structs omit — without them agents confuse event_ref/entity_ref with URNs or
// source objects.
type mcpAgentEventSelection struct {
	Ref           string `json:"ref" jsonschema:"Batch-local id referenced by nodes[].event_ref"`
	SourceCode    string `json:"source_code" jsonschema:"Gateway source_code copied from MCP results"`
	SourceEventId string `json:"source_event_id" jsonschema:"Gateway source_event_id copied from MCP results"`
}

type mcpAgentEntitySelection struct {
	Ref            string `json:"ref" jsonschema:"Batch-local id referenced by nodes[].entity_ref"`
	SourceCode     string `json:"source_code" jsonschema:"Gateway source_code copied from MCP results"`
	SourceEntityId string `json:"source_entity_id" jsonschema:"Gateway source_entity_id copied from MCP results (e.g. account:dkrylova\\administrator). Value must contain a single backslash; doubled backslashes are tolerated"`
}

type mcpAgentNode struct {
	Ref       string              `json:"ref" jsonschema:"Batch-local node id used by edges source_ref/target_ref and evidence_event_refs"`
	Why       string              `json:"why" jsonschema:"Why this node belongs on the graph: what the event/entity shows for the task"`
	EventRef  *string             `json:"event_ref,omitempty" jsonschema:"Batch-local events[].ref from this same request — not a URN and not a source object"`
	EntityRef *string             `json:"entity_ref,omitempty" jsonschema:"Batch-local entities[].ref from this same request — not a URN and not a source object"`
	EventId   *openapi_types.UUID `json:"event_id,omitempty" jsonschema:"UUID of an event already attached to this investigation"`
	EntityId  *openapi_types.UUID `json:"entity_id,omitempty" jsonschema:"UUID of an entity already attached to this investigation"`
	NodeId    *openapi_types.UUID `json:"node_id,omitempty" jsonschema:"UUID of an existing graph node"`
}

type mcpAgentEdge struct {
	SourceRef         string   `json:"source_ref" jsonschema:"Batch-local nodes[].ref of the edge source"`
	TargetRef         string   `json:"target_ref" jsonschema:"Batch-local nodes[].ref of the edge target"`
	RelationCode      string   `json:"relation_code" jsonschema:"Relation code from get_investigation_reference matching source/target kinds"`
	Why               string   `json:"why" jsonschema:"Concise evidence-based rationale; stored as proposed for analyst review"`
	Confidence        *float32 `json:"confidence,omitempty" jsonschema:"Optional confidence from 0 to 1"`
	EvidenceEventRefs []string `json:"evidence_event_refs" jsonschema:"Batch-local nodes[].ref values of event nodes from this same batch"`
}

type addAgentResultsArgs struct {
	InvestigationID      openapi_types.UUID                   `json:"investigation_id" jsonschema:"Investigation UUID"`
	HypothesisID         *openapi_types.UUID                  `json:"hypothesis_id,omitempty" jsonschema:"Optional active hypothesis UUID; when set, add explicit results to its graph projection"`
	SomIssueIds          []openapi_types.UUID                 `json:"som_issue_ids" jsonschema:"SOM issue UUIDs that produced these results"`
	Events               []mcpAgentEventSelection             `json:"events" jsonschema:"Gateway events to import; empty when nodes use event_id or node_id only"`
	Entities             []mcpAgentEntitySelection            `json:"entities" jsonschema:"Gateway entities to import; empty when nodes use entity_id or node_id only"`
	Nodes                []mcpAgentNode                       `json:"nodes" jsonschema:"Each node needs ref plus exactly one locator: event_ref, entity_ref, event_id, entity_id, or node_id"`
	Edges                []mcpAgentEdge                       `json:"edges" jsonschema:"Proposed evidence-backed edges between nodes in this batch"`
	EntityGroupProposals *[]investigations.AgentGroupProposal `json:"entity_group_proposals,omitempty" jsonschema:"Optional proposed resolved_entity groups using batch-local node refs; proposals never confirm grouping"`
	EventGroupProposals  *[]investigations.AgentGroupProposal `json:"event_group_proposals,omitempty" jsonschema:"Optional proposed event groups using batch-local node refs; proposals never confirm grouping"`
}

func (args addAgentResultsArgs) toBatch() investigations.AgentResultBatch {
	batch := investigations.AgentResultBatch{
		SomIssueIds:          args.SomIssueIds,
		Events:               make([]investigations.AgentEventSelection, len(args.Events)),
		Entities:             make([]investigations.AgentEntitySelection, len(args.Entities)),
		Nodes:                make([]investigations.AgentNode, len(args.Nodes)),
		Edges:                make([]investigations.AgentEdge, len(args.Edges)),
		EntityGroupProposals: args.EntityGroupProposals,
		EventGroupProposals:  args.EventGroupProposals,
	}
	for i, event := range args.Events {
		batch.Events[i] = investigations.AgentEventSelection{
			Ref: event.Ref, SourceCode: event.SourceCode, SourceEventId: event.SourceEventId,
		}
	}
	for i, entity := range args.Entities {
		batch.Entities[i] = investigations.AgentEntitySelection{
			Ref: entity.Ref, SourceCode: entity.SourceCode, SourceEntityId: entity.SourceEntityId,
		}
	}
	for i, node := range args.Nodes {
		batch.Nodes[i] = investigations.AgentNode{
			Ref: node.Ref, Why: node.Why, EventRef: node.EventRef, EntityRef: node.EntityRef,
			EventId: node.EventId, EntityId: node.EntityId, NodeId: node.NodeId,
		}
	}
	for i, edge := range args.Edges {
		batch.Edges[i] = investigations.AgentEdge{
			SourceRef: edge.SourceRef, TargetRef: edge.TargetRef, RelationCode: edge.RelationCode,
			Why: edge.Why, Confidence: edge.Confidence, EvidenceEventRefs: edge.EvidenceEventRefs,
		}
	}
	return batch
}

func (s *Server) MCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sb0rka-investigation",
		Version: "1.0.0",
	}, nil)
	mcp.AddTool[investigationArgs, any](server, mcpTool[investigationArgs](
		"get_investigation_graph",
		"Read this investigation or hypothesis graph, including proposed agent edges. projection defaults to raw; grouped folds confirmed tree-local groups without importing sibling evidence.",
	), s.getInvestigationGraphTool)
	mcp.AddTool[listEventsArgs, any](server, mcpTool[listEventsArgs](
		"list_investigation_events",
		"Read one page of the investigation timeline.",
	), s.listInvestigationEventsTool)
	mcp.AddTool[addAgentResultsArgs, any](server, mcpTool[addAgentResultsArgs](
		"add_investigation_agent_results",
		"Atomically add graph nodes, proposed evidence-backed edges, and optional entity/event group proposals. "+
			"Group proposals require batch-local node references, evidence, reasons and SOM provenance; they never confirm grouping. "+
			"Optionally scope the nodes to an active hypothesis. "+
			"Prefer import_entity_events when the task is to find events for one entity and put them on the graph. "+
			"To import Gateway evidence manually: put events[{ref,source_code,source_event_id}] and entities[{ref,source_code,source_entity_id}], "+
			"then nodes use event_ref/entity_ref equal to those batch-local refs (never URNs or {source_code,...} objects). "+
			"Already-attached evidence uses event_id/entity_id/node_id instead. Never put an IR UUID into source_entity_id/source_event_id. "+
			"Example: events:[{ref:\"e0\",source_code:\"mock\",source_event_id:\"evt-1\"}], "+
			"entities:[{ref:\"a0\",source_code:\"mock\",source_entity_id:\"ent-1\"}], "+
			"nodes:[{ref:\"n-event\",why:\"matched task evidence\",event_ref:\"e0\"},{ref:\"n-entity\",why:\"task target\",entity_ref:\"a0\"}], "+
			"edges:[{source_ref:\"n-event\",target_ref:\"n-entity\",relation_code:\"actor\",why:\"…\",evidence_event_refs:[\"n-event\"]}].",
	), s.addInvestigationAgentResultsTool)
	mcp.AddTool[importEntityEventsArgs, any](server, mcpTool[importEntityEventsArgs](
		"import_entity_events",
		"Find Gateway events for one entity and import them onto the investigation graph with proposed role edges. "+
			"Pass entity.entity_id when the issue/graph already has an IR UUID, or entity.type+entity.value otherwise. "+
			"If the issue names a predicate or order, pass it via filter/sort instead of hand-building a Gateway batch. "+
			"Never put an IR UUID into type/value. Prefer this over manually assembling add_investigation_agent_results for the common enrich-entity workflow. "+
			"Example: entity:{entity_id:\"b71336ed-25f7-42fa-840a-688ceb087c74\"}, time_range optional, limit defaults to 50.",
	), s.importEntityEventsTool)
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
	if args.Projection != "" && args.Projection != "raw" && args.Projection != "grouped" {
		return mcpFailure(errors.New("invalid arguments: projection must be raw or grouped"))
	}
	if args.Projection == "grouped" {
		value, err := s.groupProjection(ctx, id, args.HypothesisID, false, nil, nil)
		if err != nil {
			return mcpFailure(err)
		}
		return nil, value, nil
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

	body := investigations.AddAgentResultsJSONRequestBody(args.toBatch())
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

// MCP-facing Gateway entity conditions spell out the IR-vs-source ID split that
// generated OpenAPI structs leave undescribed.
type mcpEntityRef struct {
	Type  string `json:"type" jsonschema:"Canonical kind from graph/timeline/Gateway results: account, host, ip, domain, … — never an IR entity UUID"`
	Value string `json:"value" jsonschema:"Entity value such as dkrylova\\administrator — never an investigation entity_id. Value must contain a single backslash; doubled backslashes are tolerated"`
}

type mcpSearchEventsArgs struct {
	Sources           *[]string                    `json:"sources,omitempty" jsonschema:"Codes from gateway_list_sources. Match capability: accounts/process/auth → SIEM (pt-maxpatrol-siem); network sessions → NAD"`
	TimeRange         gatewaycontract.TimeRange    `json:"time_range" jsonschema:"Required occurrence-time interval"`
	Entities          *[]mcpEntityRef              `json:"entities,omitempty" jsonschema:"Prefer this to find events for an account/host/ip. Never pass IR entity UUIDs here"`
	Filter            *string                      `json:"filter,omitempty" jsonschema:"Optional SIEM predicate. Prefer entities[] when filtering by identity"`
	Columns           *[]string                    `json:"columns,omitempty" jsonschema:"Optional allowlisted SIEM fields to expose"`
	Sort              *[]gatewaycontract.EventSort `json:"sort,omitempty" jsonschema:"Optional SIEM sort rules"`
	GroupBy           *[]string                    `json:"group_by,omitempty" jsonschema:"Optional SIEM group_by fields when drilling into an aggregation"`
	GroupValues       *[]*string                   `json:"group_values,omitempty" jsonschema:"Group values aligned with group_by"`
	Limit             *int                         `json:"limit,omitempty" jsonschema:"Max merged events per page (1-100). Defaults to 20 when omitted"`
	Cursor            *string                      `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from the previous page with the same filters"`
	IncludeAttributes *bool                        `json:"include_attributes,omitempty" jsonschema:"When true, keep event.attributes in the response. Default false to keep pages small"`
}

func (a mcpSearchEventsArgs) toContract() gatewaycontract.SearchEventsRequest {
	limit := a.Limit
	if limit == nil {
		defaultLimit := 20
		limit = &defaultLimit
	}
	out := gatewaycontract.SearchEventsRequest{
		Sources: a.Sources, TimeRange: a.TimeRange, Filter: a.Filter, Columns: a.Columns,
		Sort: a.Sort, GroupBy: a.GroupBy, GroupValues: a.GroupValues, Limit: limit, Cursor: a.Cursor,
	}
	if a.Entities != nil {
		entities := make([]gatewaycontract.EntityRef, len(*a.Entities))
		for i, entity := range *a.Entities {
			entities[i] = gatewaycontract.EntityRef{
				Type:  entity.Type,
				Value: normalizeEntityValue(entity.Type, entity.Value),
			}
		}
		out.Entities = &entities
	}
	return out
}

type mcpLookupEntityArgs struct {
	Entity    mcpEntityRef              `json:"entity" jsonschema:"Entity to enrich using type+value from MCP results — not an IR UUID"`
	Sources   *[]string                 `json:"sources,omitempty" jsonschema:"Codes from gateway_list_sources; omit to fan out to every allowed lookup source"`
	TimeRange gatewaycontract.TimeRange `json:"time_range" jsonschema:"Required occurrence-time interval for source records"`
}

func (a mcpLookupEntityArgs) toContract() gatewaycontract.LookupEntityRequest {
	return gatewaycontract.LookupEntityRequest{
		Entity: gatewaycontract.EntityRef{
			Type:  a.Entity.Type,
			Value: normalizeEntityValue(a.Entity.Type, a.Entity.Value),
		},
		Sources:   a.Sources,
		TimeRange: a.TimeRange,
	}
}

type importEntitySelector struct {
	EntityID *openapi_types.UUID `json:"entity_id,omitempty" jsonschema:"IR entity UUID already known from the issue or investigation — preferred"`
	Type     *string             `json:"type,omitempty" jsonschema:"Canonical entity kind when entity_id is unknown — never an IR UUID"`
	Value    *string             `json:"value,omitempty" jsonschema:"Entity value such as dkrylova\\administrator when entity_id is unknown"`
}

type importEntityEventsArgs struct {
	InvestigationID     openapi_types.UUID           `json:"investigation_id" jsonschema:"Investigation UUID"`
	HypothesisID        *openapi_types.UUID          `json:"hypothesis_id,omitempty" jsonschema:"Optional active hypothesis UUID"`
	SomIssueIds         []openapi_types.UUID         `json:"som_issue_ids" jsonschema:"SOM issue UUIDs that produced these results"`
	Entity              importEntitySelector         `json:"entity" jsonschema:"Exactly one of entity_id or type+value"`
	TimeRange           *gatewaycontract.TimeRange   `json:"time_range,omitempty" jsonschema:"Optional search window; defaults to investigation timeline ±24h or last 30 days"`
	Sources             *[]string                    `json:"sources,omitempty" jsonschema:"Optional source codes; defaults by entity capability (SIEM for account/host/process)"`
	Filter              *string                      `json:"filter,omitempty" jsonschema:"Bounded SIEM predicate, e.g. correlation_name != null; combined with the entity condition"`
	Sort                *[]gatewaycontract.EventSort `json:"sort,omitempty" jsonschema:"Defaults to time desc — the newest events; use time asc for the start of the window"`
	Limit               *int                         `json:"limit,omitempty" jsonschema:"Max events to import (1-100, default 50)"`
	IncludeParticipants *bool                        `json:"include_participants,omitempty" jsonschema:"When true, also import other entities mentioned on the events. Default false"`
}

type importEntityEventsSummary struct {
	Entity           importEntityEventsEntitySummary `json:"entity"`
	EventsFound      int                             `json:"events_found"`
	EventsImported   int                             `json:"events_imported"`
	EventsTotal      *int64                          `json:"events_total,omitempty"`
	EventsTotalExact bool                            `json:"events_total_exact"`
	Truncated        bool                            `json:"truncated"`
	Filter           *string                         `json:"filter,omitempty"`
	Sort             *[]gatewaycontract.EventSort    `json:"sort,omitempty"`
	NextCursor       *string                         `json:"next_cursor,omitempty"`
	SourceStates     []gatewaycontract.SourceState   `json:"source_states,omitempty"`
	SourceErrors     []gatewaycontract.SourceError   `json:"source_errors,omitempty"`
}

type importEntityEventsEntitySummary struct {
	EntityID *string `json:"entity_id,omitempty"`
	Type     string  `json:"type"`
	Value    string  `json:"value"`
}

type importEntityEventsResult struct {
	investigations.ContextImportResult
	Summary importEntityEventsSummary `json:"summary"`
}

func (s *Server) importEntityEventsTool(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args importEntityEventsArgs,
) (*mcp.CallToolResult, any, error) {
	investigationID, err := requireMCPInvestigation(args.InvestigationID)
	if err != nil {
		return mcpFailure(err)
	}
	if err := requireMCPHypothesis(args.HypothesisID); err != nil {
		return mcpFailure(err)
	}
	if len(args.SomIssueIds) == 0 {
		return mcpFailure(errors.New("invalid arguments: expected at least one SOM issue"))
	}
	for _, id := range args.SomIssueIds {
		if id == uuid.Nil {
			return mcpFailure(errors.New("invalid arguments: som_issue_ids must contain non-zero UUIDs"))
		}
	}
	limit := 50
	if args.Limit != nil {
		limit = *args.Limit
	}
	if limit < 1 || limit > 100 {
		return mcpFailure(errors.New("invalid arguments: limit must be between 1 and 100"))
	}
	includeParticipants := args.IncludeParticipants != nil && *args.IncludeParticipants

	scope, err := s.scope(ctx)
	if err != nil {
		return mcpFailure(err)
	}
	bearer, ok := socctx.BearerFromContext(ctx)
	if !ok {
		return mcpFailure(httperr.ErrUnauthorized)
	}
	if _, err := s.db.GetInvestigation(ctx, scope.ProjectID, investigationID.String()); err != nil {
		return mcpFailure(err)
	}
	if args.HypothesisID != nil {
		hypothesis, err := s.db.GetHypothesis(ctx, scope.ProjectID, investigationID.String(), args.HypothesisID.String())
		if err != nil {
			return mcpFailure(err)
		}
		if hypothesis.Status != "active" {
			return mcpFailure(hypothesisStoreError(&store.ConflictError{IDs: []string{args.HypothesisID.String()}}))
		}
	}

	resolvedEntity, err := s.resolveImportEntity(ctx, scope.ProjectID, investigationID.String(), args.Entity)
	if err != nil {
		return mcpFailure(err)
	}
	timeRange, err := s.resolveImportTimeRange(ctx, scope.ProjectID, investigationID.String(), args.TimeRange)
	if err != nil {
		return mcpFailure(err)
	}
	sources := args.Sources
	if sources == nil {
		defaultSources := defaultSourcesForEntityType(resolvedEntity.Type)
		if len(defaultSources) > 0 {
			sources = &defaultSources
		}
	}
	entities := &[]gatewaycontract.EntityRef{{
		Type:  resolvedEntity.Type,
		Value: resolvedEntity.Value,
	}}
	eventsTotal, eventsTotalExact, aggregateSourceErrors, aggregateWarnings := s.importEntityEventsTotal(
		ctx, scope.ProjectID, bearer, sources, timeRange, entities, args.Filter,
	)

	searchReq := gatewaycontract.SearchEventsRequest{
		Sources:   sources,
		TimeRange: timeRange,
		Entities:  entities,
		Filter:    args.Filter,
		Sort:      args.Sort,
		Limit:     &limit,
	}
	var (
		foundEvents  []gatewaycontract.Event
		pageEntities []gatewaycontract.Entity
		sourceStates []gatewaycontract.SourceState
		sourceErrors = append([]gatewaycontract.SourceError{}, aggregateSourceErrors...)
		nextCursor   *string
		truncated    bool
	)
	for {
		raw, err := s.gateway.SearchEvents(ctx, scope.ProjectID, bearer, searchReq)
		if err != nil {
			var upstream *gatewayclient.HTTPError
			if errors.As(err, &upstream) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: upstream.Message}}, IsError: true}, nil, nil
			}
			return mcpFailure(httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, "Gateway is unavailable"))
		}
		var page gatewaycontract.SearchEventsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return mcpFailure(err)
		}
		sourceStates = append(sourceStates, page.SourceStates...)
		sourceErrors = append(sourceErrors, page.SourceErrors...)
		pageEntities = append(pageEntities, page.Entities...)
		for _, event := range page.Events {
			foundEvents = append(foundEvents, event)
			if len(foundEvents) >= limit {
				break
			}
		}
		if len(foundEvents) >= limit {
			truncated = page.NextCursor != nil || len(page.Events) > 0
			nextCursor = page.NextCursor
			break
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			nextCursor = nil
			break
		}
		searchReq.Cursor = page.NextCursor
	}
	if len(foundEvents) > limit {
		foundEvents = foundEvents[:limit]
		truncated = true
	}

	summary := importEntityEventsSummary{
		Entity: importEntityEventsEntitySummary{
			Type:  resolvedEntity.Type,
			Value: resolvedEntity.Value,
		},
		EventsFound:      len(foundEvents),
		EventsTotal:      eventsTotal,
		EventsTotalExact: eventsTotalExact,
		Truncated:        truncated || eventsTotal != nil && *eventsTotal > int64(len(foundEvents)),
		Filter:           args.Filter,
		Sort:             args.Sort,
		NextCursor:       nextCursor,
		SourceStates:     sourceStates,
		SourceErrors:     sourceErrors,
	}
	if resolvedEntity.EntityID != "" {
		id := resolvedEntity.EntityID
		summary.Entity.EntityID = &id
	}

	if len(foundEvents) == 0 {
		empty := investigations.ContextImportResult{Warnings: aggregateWarnings}
		return nil, importEntityEventsResult{ContextImportResult: empty, Summary: summary}, nil
	}

	batch, err := buildEntityEventsBatch(args.SomIssueIds, resolvedEntity, foundEvents, includeParticipants, timeRange, args.Filter, args.Sort)
	if err != nil {
		return mcpImportFailure(summary, err)
	}
	var hypothesisID *string
	if args.HypothesisID != nil {
		value := args.HypothesisID.String()
		hypothesisID = &value
	}

	eventRefs := make(map[string]string, len(batch.Events))
	entityRefs := make(map[string]string, len(batch.Entities))
	eventSources := make(map[string]gatewayclient.EventSourceRef, len(batch.Events))
	entitySources := make(map[string]gatewayclient.EntitySourceRef, len(batch.Entities))
	for _, event := range batch.Events {
		ref := strings.TrimSpace(event.Ref)
		sourceKey := sourceRecordKey(event.SourceCode, event.SourceEventId)
		eventRefs[ref] = sourceKey
		eventSources[ref] = gatewayclient.EventSourceRef{SourceCode: event.SourceCode, SourceEventId: event.SourceEventId}
	}
	for _, entity := range batch.Entities {
		ref := strings.TrimSpace(entity.Ref)
		sourceEntityID := normalizeSourceEntityID(entity.SourceEntityId)
		sourceKey := sourceRecordKey(entity.SourceCode, sourceEntityID)
		entityRefs[ref] = sourceKey
		entitySources[ref] = gatewayclient.EntitySourceRef{SourceCode: entity.SourceCode, SourceEntityId: sourceEntityID}
	}

	// Persist search hits directly — re-resolving each UUID through Gateway
	// times out before a full page of SIEM events can be re-fetched.
	resolved := selectionFromSearchEvents(foundEvents, pageEntities)
	if resolvedEntity.Type != "" && resolvedEntity.Value != "" {
		markSearchEntityDirect(&resolved, resolvedEntity.Type, resolvedEntity.Value)
	}
	stats, err := s.commitAgentBatch(ctx, scope, investigationID.String(), hypothesisID, batch, eventRefs, entityRefs, eventSources, entitySources, resolved)
	if err != nil {
		return mcpImportFailure(summary, err)
	}
	result, err := importResult(stats)
	if err != nil {
		return mcpImportFailure(summary, err)
	}
	result.Warnings = append(aggregateWarnings, result.Warnings...)
	summary.EventsImported = result.Events
	return nil, importEntityEventsResult{ContextImportResult: result, Summary: summary}, nil
}

func (s *Server) importEntityEventsTotal(
	ctx context.Context,
	projectID, bearer string,
	sources *[]string,
	timeRange gatewaycontract.TimeRange,
	entities *[]gatewaycontract.EntityRef,
	filter *string,
) (*int64, bool, []gatewaycontract.SourceError, []string) {
	groupLimit := 50
	raw, err := s.gateway.AggregateEvents(ctx, projectID, bearer, gatewaycontract.AggregateEventsRequest{
		Sources: sources, TimeRange: timeRange, Entities: entities, Filter: filter,
		GroupBy: []string{"correlation_type"}, Limit: &groupLimit,
	})
	if err != nil {
		return nil, false, nil, []string{"events_total unavailable: " + err.Error()}
	}
	var aggregate gatewaycontract.AggregateEventsResponse
	if err := json.Unmarshal(raw, &aggregate); err != nil {
		return nil, false, nil, []string{"events_total unavailable: invalid Gateway aggregate response"}
	}
	var total int64
	for _, group := range aggregate.Groups {
		total += group.Count
	}
	exact := len(aggregate.Groups) < groupLimit && len(aggregate.SourceErrors) == 0
	return &total, exact, aggregate.SourceErrors, nil
}

func markSearchEntityDirect(resolved *resolvedGatewayContext, typeCode, value string) {
	snapshotID := entityKey(typeCode, normalizeEntityValue(typeCode, value))
	for i := range resolved.Selection.Entities {
		if resolved.Selection.Entities[i].SnapshotID == snapshotID {
			resolved.Selection.Entities[i].Direct = true
			return
		}
	}
}

func mcpImportFailure(summary importEntityEventsSummary, err error) (*mcp.CallToolResult, any, error) {
	message := "internal server error"
	var domain *httperr.Error
	if errors.As(err, &domain) {
		message = domain.Message
	} else if err != nil && strings.HasPrefix(err.Error(), "invalid arguments") {
		message = err.Error()
	} else if err != nil {
		message = err.Error()
	}
	summary.EventsImported = 0
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}, importEntityEventsResult{Summary: summary}, nil
}

type resolvedImportEntity struct {
	EntityID string
	Type     string
	Value    string
	Sources  []model.EntitySource
	Attached bool
}

func (s *Server) resolveImportEntity(ctx context.Context, projectID, investigationID string, selector importEntitySelector) (resolvedImportEntity, error) {
	hasID := selector.EntityID != nil && *selector.EntityID != uuid.Nil
	hasTypeValue := selector.Type != nil && strings.TrimSpace(*selector.Type) != "" &&
		selector.Value != nil && strings.TrimSpace(*selector.Value) != ""
	if hasID == hasTypeValue {
		return resolvedImportEntity{}, errors.New("invalid arguments: entity requires exactly one of entity_id or type+value")
	}
	if hasID {
		card, err := s.db.GetEntityCard(ctx, projectID, selector.EntityID.String())
		if err != nil {
			return resolvedImportEntity{}, storeError(err)
		}
		attached := false
		for _, occurrence := range card.Occurrences {
			if occurrence.InvestigationID == investigationID {
				attached = true
				break
			}
		}
		if !attached {
			return resolvedImportEntity{}, httperr.ErrNotFound
		}
		value := card.Entity.CanonicalKey
		if card.Entity.DisplayName != nil && strings.TrimSpace(*card.Entity.DisplayName) != "" {
			value = *card.Entity.DisplayName
		}
		return resolvedImportEntity{
			EntityID: card.Entity.ID,
			Type:     card.Entity.TypeCode,
			Value:    normalizeEntityValue(card.Entity.TypeCode, value),
			Sources:  card.Entity.Sources,
			Attached: true,
		}, nil
	}
	typeCode := strings.TrimSpace(*selector.Type)
	value := normalizeEntityValue(typeCode, *selector.Value)
	if looksLikeBareUUID(value) {
		return resolvedImportEntity{}, errors.New("invalid arguments: entity.value must not be an IR UUID; use entity.entity_id")
	}
	return resolvedImportEntity{Type: typeCode, Value: value}, nil
}

func (s *Server) resolveImportTimeRange(
	ctx context.Context,
	projectID, investigationID string,
	override *gatewaycontract.TimeRange,
) (gatewaycontract.TimeRange, error) {
	if override != nil {
		if override.From.IsZero() || !override.From.Before(override.To) {
			return gatewaycontract.TimeRange{}, errors.New("invalid arguments: time_range.from must be earlier than time_range.to")
		}
		return *override, nil
	}
	from, to, err := s.db.InvestigationTimelineBounds(ctx, projectID, investigationID)
	if err != nil {
		return gatewaycontract.TimeRange{}, storeError(err)
	}
	if from != nil && to != nil && !from.IsZero() && !to.IsZero() {
		return gatewaycontract.TimeRange{
			From: from.Add(-24 * time.Hour),
			To:   to.Add(24 * time.Hour),
		}, nil
	}
	now := time.Now().UTC()
	return gatewaycontract.TimeRange{From: now.Add(-30 * 24 * time.Hour), To: now}, nil
}

func defaultSourcesForEntityType(typeCode string) []string {
	switch strings.ToLower(strings.TrimSpace(typeCode)) {
	case "account", "host", "hostname", "process", "hash", "file_hash", "md5", "sha1", "sha256":
		return []string{"pt-maxpatrol-siem"}
	case "ip", "domain":
		return nil
	default:
		return []string{"pt-maxpatrol-siem"}
	}
}

func buildEntityEventsBatch(
	somIssueIDs []openapi_types.UUID,
	entity resolvedImportEntity,
	events []gatewaycontract.Event,
	includeParticipants bool,
	timeRange gatewaycontract.TimeRange,
	filter *string,
	sortRules *[]gatewaycontract.EventSort,
) (investigations.AgentResultBatch, error) {
	batch := investigations.AgentResultBatch{
		SomIssueIds: somIssueIDs,
		Events:      make([]investigations.AgentEventSelection, 0, len(events)),
		Entities:    nil,
		Nodes:       nil,
		Edges:       nil,
	}
	entityNodeRef := "n-entity"
	entitySelectionRef := "a0"
	confidence := float32(1)
	targetWhy := fmt.Sprintf("target entity of import_entity_events (issue %s)", formatIssueIDs(somIssueIDs))

	if entity.Attached && entity.EntityID != "" {
		entityUUID, err := uuid.Parse(entity.EntityID)
		if err != nil {
			return investigations.AgentResultBatch{}, err
		}
		id := openapi_types.UUID(entityUUID)
		batch.Nodes = append(batch.Nodes, investigations.AgentNode{Ref: entityNodeRef, Why: targetWhy, EntityId: &id})
	} else {
		sourceCode, sourceEntityID := pickEntitySource(entity, events)
		if sourceCode == "" || sourceEntityID == "" {
			return investigations.AgentResultBatch{}, errors.New("invalid arguments: could not determine source_entity_id for the target entity")
		}
		batch.Entities = append(batch.Entities, investigations.AgentEntitySelection{
			Ref: entitySelectionRef, SourceCode: sourceCode, SourceEntityId: normalizeSourceEntityID(sourceEntityID),
		})
		ref := entitySelectionRef
		batch.Nodes = append(batch.Nodes, investigations.AgentNode{Ref: entityNodeRef, Why: targetWhy, EntityRef: &ref})
	}

	seenEvents := map[string]struct{}{}
	participantRefs := map[string]string{} // type\x00value -> entity selection ref
	for i, event := range events {
		key := event.SourceCode + "\x00" + event.SourceEventId
		if _, ok := seenEvents[key]; ok {
			continue
		}
		seenEvents[key] = struct{}{}
		eventRef := fmt.Sprintf("e%d", i)
		nodeRef := fmt.Sprintf("n-event-%d", i)
		batch.Events = append(batch.Events, investigations.AgentEventSelection{
			Ref: eventRef, SourceCode: event.SourceCode, SourceEventId: event.SourceEventId,
		})
		batch.Nodes = append(batch.Nodes, investigations.AgentNode{
			Ref: nodeRef, Why: importEventNodeWhy(entity, event, timeRange, filter, sortRules), EventRef: &eventRef,
		})

		matched := false
		for _, mention := range event.Entities {
			mentionValue := normalizeEntityValue(mention.Type, mention.Value)
			isTarget := strings.EqualFold(mention.Type, entity.Type) &&
				strings.EqualFold(mentionValue, entity.Value)
			if isTarget {
				matched = true
				roles := mention.Roles
				if len(roles) == 0 {
					roles = []gatewaycontract.EntityMentionRoles{gatewaycontract.Mentions}
				}
				for _, role := range roles {
					batch.Edges = append(batch.Edges, investigations.AgentEdge{
						SourceRef: nodeRef, TargetRef: entityNodeRef, RelationCode: string(role),
						Why: fmt.Sprintf("role %s of %s observed in %s event %s at %s",
							role, entity.Value, event.SourceCode, event.SourceEventId, event.OccurredAt.UTC().Format(time.RFC3339)),
						Confidence:        &confidence,
						EvidenceEventRefs: []string{nodeRef},
					})
				}
				continue
			}
			if !includeParticipants {
				continue
			}
			partKey := strings.ToLower(mention.Type) + "\x00" + mentionValue
			partRef, ok := participantRefs[partKey]
			if !ok {
				partRef = fmt.Sprintf("a%d", len(participantRefs)+1)
				participantRefs[partKey] = partRef
				sourceEntityID := mention.Type + ":" + mentionValue
				batch.Entities = append(batch.Entities, investigations.AgentEntitySelection{
					Ref: partRef, SourceCode: event.SourceCode, SourceEntityId: normalizeSourceEntityID(sourceEntityID),
				})
				partNode := "n-" + partRef
				batch.Nodes = append(batch.Nodes, investigations.AgentNode{
					Ref: partNode,
					Why: fmt.Sprintf("participant entity %s %s observed in %s event %s imported by import_entity_events",
						mention.Type, mentionValue, event.SourceCode, event.SourceEventId),
					EntityRef: &partRef,
				})
				participantRefs[partKey+"\x00node"] = partNode
			}
			partNode := participantRefs[partKey+"\x00node"]
			roles := mention.Roles
			if len(roles) == 0 {
				roles = []gatewaycontract.EntityMentionRoles{gatewaycontract.Mentions}
			}
			for _, role := range roles {
				batch.Edges = append(batch.Edges, investigations.AgentEdge{
					SourceRef: nodeRef, TargetRef: partNode, RelationCode: string(role),
					Why: fmt.Sprintf("role %s of %s observed in %s event %s at %s",
						role, mentionValue, event.SourceCode, event.SourceEventId, event.OccurredAt.UTC().Format(time.RFC3339)),
					Confidence:        &confidence,
					EvidenceEventRefs: []string{nodeRef},
				})
			}
		}
		if !matched {
			// Still connect the target entity with a generic mentions edge so the
			// search hit is not left as an isolated event node.
			batch.Edges = append(batch.Edges, investigations.AgentEdge{
				SourceRef: nodeRef, TargetRef: entityNodeRef, RelationCode: string(gatewaycontract.Mentions),
				Why: fmt.Sprintf("entity %s matched search for event %s from %s at %s",
					entity.Value, event.SourceEventId, event.SourceCode, event.OccurredAt.UTC().Format(time.RFC3339)),
				Confidence:        &confidence,
				EvidenceEventRefs: []string{nodeRef},
			})
		}
	}
	return batch, nil
}

func formatIssueIDs(ids []openapi_types.UUID) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return strings.Join(values, ", ")
}

func importEventNodeWhy(
	entity resolvedImportEntity,
	event gatewaycontract.Event,
	timeRange gatewaycontract.TimeRange,
	filter *string,
	sortRules *[]gatewaycontract.EventSort,
) string {
	roles := make([]string, 0)
	for _, mention := range event.Entities {
		if !strings.EqualFold(mention.Type, entity.Type) ||
			!strings.EqualFold(normalizeEntityValue(mention.Type, mention.Value), entity.Value) {
			continue
		}
		if len(mention.Roles) == 0 {
			roles = append(roles, string(gatewaycontract.Mentions))
			continue
		}
		for _, role := range mention.Roles {
			roles = append(roles, string(role))
		}
	}
	if len(roles) == 0 {
		roles = append(roles, string(gatewaycontract.Mentions))
	}
	return fmt.Sprintf(
		"matched import_entity_events for %s %s in %s, window %s..%s, filter %s, sort %s, role %s",
		entity.Type, entity.Value, event.SourceCode,
		timeRange.From.UTC().Format(time.RFC3339), timeRange.To.UTC().Format(time.RFC3339),
		formatImportFilter(filter), formatImportSort(sortRules), strings.Join(roles, ","),
	)
}

func formatImportFilter(filter *string) string {
	if filter == nil || strings.TrimSpace(*filter) == "" {
		return "none"
	}
	return fmt.Sprintf("%q", strings.TrimSpace(*filter))
}

func formatImportSort(rules *[]gatewaycontract.EventSort) string {
	if rules == nil || len(*rules) == 0 {
		return "time desc (default)"
	}
	values := make([]string, 0, len(*rules))
	for _, rule := range *rules {
		values = append(values, strings.TrimSpace(rule.Field)+" "+string(rule.Direction))
	}
	return strings.Join(values, ",")
}

func pickEntitySource(entity resolvedImportEntity, events []gatewaycontract.Event) (sourceCode, sourceEntityID string) {
	for _, source := range entity.Sources {
		if strings.TrimSpace(source.SourceCode) != "" && strings.TrimSpace(source.SourceEntityID) != "" {
			return source.SourceCode, source.SourceEntityID
		}
	}
	for _, event := range events {
		for _, mention := range event.Entities {
			if strings.EqualFold(mention.Type, entity.Type) &&
				strings.EqualFold(normalizeEntityValue(mention.Type, mention.Value), entity.Value) {
				return event.SourceCode, mention.Type + ":" + normalizeEntityValue(mention.Type, mention.Value)
			}
		}
	}
	if entity.Type != "" && entity.Value != "" && len(events) > 0 {
		return events[0].SourceCode, entity.Type + ":" + entity.Value
	}
	return "", ""
}

func addGatewayTools(server *mcp.Server, s *Server) {
	mcp.AddTool(server, mcpTool[struct{}](
		"gateway_list_sources", "List project-allowed Gateway sources and their capabilities. Use capabilities to pick SIEM vs NAD before searching.",
	), gatewayHandler(s, func(ctx context.Context, _ struct{}, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.ListSources(ctx, scope.ProjectID, bearer)
	}))
	mcp.AddTool(server, mcpTool[mcpSearchEventsArgs](
		"gateway_search_events",
		"Search normalized events across project-allowed sources. "+
			"Filter identities with entities:[{type,value}] (e.g. account + dkrylova\\administrator) — never IR entity_id UUIDs. "+
			"Pick sources by capability (accounts/process/auth → pt-maxpatrol-siem, not NAD). "+
			"Default limit is 20; attributes are omitted unless include_attributes=true. "+
			"Empty page with truncated source_states is not proof of absence: follow next_cursor or narrow time_range/filters and retry. "+
			"To verify one known source_event_id use gateway_resolve_context, not a search filter.",
	), gatewayHandler(s, func(ctx context.Context, args mcpSearchEventsArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		raw, err := s.gateway.SearchEvents(ctx, scope.ProjectID, bearer, args.toContract())
		if err != nil {
			return nil, err
		}
		if args.IncludeAttributes != nil && *args.IncludeAttributes {
			return raw, nil
		}
		return slimSearchEventsResponse(raw)
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.AggregateEventsRequest](
		"gateway_aggregate_events", "Group and count events using source-supported fields.",
	), gatewayHandler(s, func(ctx context.Context, args gatewaycontract.AggregateEventsRequest, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.AggregateEvents(ctx, scope.ProjectID, bearer, args)
	}))
	mcp.AddTool(server, mcpTool[mcpLookupEntityArgs](
		"gateway_lookup_entity",
		"Enrich one entity through project-allowed sources. "+
			"Pass entity.type + entity.value from graph/timeline/Gateway results (e.g. type=account, value=dkrylova\\administrator). "+
			"Never pass an investigation entity_id UUID as value. Prefer SIEM sources for accounts; NAD is for network identities.",
	), gatewayHandler(s, func(ctx context.Context, args mcpLookupEntityArgs, scope socctx.Scope, bearer string) (json.RawMessage, error) {
		return s.gateway.LookupEntity(ctx, scope.ProjectID, bearer, args.toContract())
	}))
	mcp.AddTool(server, mcpTool[gatewaycontract.ResolveContextRequest](
		"gateway_resolve_context",
		"Resolve selected finding, session, event, or entity references into normalized context. Read-only: persist only explicitly selected events and entities through add_investigation_agent_results or import_entity_events. Use source_code/source_*_id refs — not IR UUIDs. Prefer this to verify one known source_event_id.",
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

func slimSearchEventsResponse(raw json.RawMessage) (json.RawMessage, error) {
	var page gatewaycontract.SearchEventsResponse
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	for i := range page.Events {
		page.Events[i].Attributes = nil
	}
	return json.Marshal(page)
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
