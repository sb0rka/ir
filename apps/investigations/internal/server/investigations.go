package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

func (s *Server) ListInvestigations(ctx context.Context, request investigations.ListInvestigationsRequestObject) (investigations.ListInvestigationsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "limit must be between 1 and 200")
	}
	cursor, err := decodeCursor(request.Params.Cursor)
	if err != nil {
		return nil, err
	}
	filter := model.InvestigationFilter{Limit: limit + 1, RootsOnly: request.Params.ParentId == nil, Cursor: cursor}
	if request.Params.ParentId != nil {
		id := request.Params.ParentId.String()
		filter.ParentID = &id
		filter.RootsOnly = false
	}
	if request.Params.Status != nil {
		v := string(*request.Params.Status)
		filter.Status = &v
	}
	if request.Params.Severity != nil {
		v := string(*request.Params.Severity)
		filter.Severity = &v
	}
	if request.Params.Q != nil {
		v := strings.TrimSpace(*request.Params.Q)
		if v != "" {
			filter.Q = &v
		}
	}
	items, err := s.db.ListInvestigations(ctx, scope.ProjectID, filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := investigations.InvestigationPage{Investigations: make([]investigations.Investigation, 0, min(len(items), limit))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertInvestigation(item)
		if err != nil {
			return nil, err
		}
		page.Investigations = append(page.Investigations, converted)
	}
	return investigations.ListInvestigations200JSONResponse(page), nil
}

func (s *Server) CreateInvestigation(ctx context.Context, request investigations.CreateInvestigationRequestObject) (investigations.CreateInvestigationResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	title := strings.TrimSpace(request.Body.Title)
	if title == "" || len(title) > 255 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "title must be 1-255 characters")
	}
	if len(request.Body.SomWorkspaceIds) == 0 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "som_workspace_ids must not be empty")
	}
	input := model.InvestigationNew{ProjectID: scope.ProjectID, Title: title, Description: request.Body.Description, Severity: (*string)(request.Body.Severity)}
	if request.Body.ParentId != nil {
		id := request.Body.ParentId.String()
		input.ParentID = &id
	}
	for _, id := range request.Body.SomWorkspaceIds {
		input.WorkspaceIDs = append(input.WorkspaceIDs, id.String())
	}
	created, err := s.db.CreateInvestigation(ctx, input)
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertInvestigation(created)
	if err != nil {
		return nil, err
	}
	return investigations.CreateInvestigation201JSONResponse(out), nil
}

func (s *Server) GetInvestigation(ctx context.Context, request investigations.GetInvestigationRequestObject) (investigations.GetInvestigationResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetInvestigation(ctx, scope.ProjectID, request.InvestigationId.String())
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertInvestigation(item)
	if err != nil {
		return nil, err
	}
	return investigations.GetInvestigation200JSONResponse(out), nil
}

func (s *Server) AddInvestigationContext(ctx context.Context, request investigations.AddInvestigationContextRequestObject) (investigations.AddInvestigationContextResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if _, err := s.db.GetInvestigation(ctx, scope.ProjectID, request.InvestigationId.String()); err != nil {
		return nil, storeError(err)
	}
	gatewayRequest := gatewayclient.ResolveContextRequest{}
	for _, ref := range request.Body.Events {
		gatewayRequest.Events = append(gatewayRequest.Events, gatewayclient.EventSourceRef{SourceCode: ref.SourceCode, SourceEventId: ref.SourceEventId})
	}
	for _, ref := range request.Body.Entities {
		gatewayRequest.Entities = append(gatewayRequest.Entities, gatewayclient.EntitySourceRef{SourceCode: ref.SourceCode, SourceEntityId: ref.SourceEntityId})
	}
	resolved, err := s.resolveGatewayContext(ctx, scope, gatewayRequest)
	if err != nil {
		return nil, err
	}
	stats, err := s.db.ImportContext(ctx, model.ImportRequest{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(), Selection: resolved.Selection, Origin: "analyst"})
	if err != nil {
		return nil, storeError(err)
	}
	return investigations.AddInvestigationContext201JSONResponse(importResult(stats)), nil
}

func (s *Server) AddAgentResults(ctx context.Context, request investigations.AddAgentResultsRequestObject) (investigations.AddAgentResultsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if len(request.Body.SomIssueIds) == 0 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "som_issue_ids must not be empty")
	}
	if _, err := s.db.GetInvestigation(ctx, scope.ProjectID, request.InvestigationId.String()); err != nil {
		return nil, storeError(err)
	}
	gatewayRequest := gatewayclient.ResolveContextRequest{}
	eventRefs := make(map[string]string, len(request.Body.Events))
	entityRefs := make(map[string]string, len(request.Body.Entities))
	for _, event := range request.Body.Events {
		ref := strings.TrimSpace(event.Ref)
		if ref == "" {
			return nil, validationError("event ref is required")
		}
		if _, duplicate := eventRefs[ref]; duplicate {
			return nil, validationError("event refs must be unique")
		}
		sourceKey := sourceRecordKey(event.SourceCode, event.SourceEventId)
		eventRefs[ref] = sourceKey
		gatewayRequest.Events = append(gatewayRequest.Events, gatewayclient.EventSourceRef{SourceCode: event.SourceCode, SourceEventId: event.SourceEventId})
	}
	for _, entity := range request.Body.Entities {
		ref := strings.TrimSpace(entity.Ref)
		if ref == "" {
			return nil, validationError("entity ref is required")
		}
		if _, duplicate := entityRefs[ref]; duplicate {
			return nil, validationError("entity refs must be unique")
		}
		sourceKey := sourceRecordKey(entity.SourceCode, entity.SourceEntityId)
		entityRefs[ref] = sourceKey
		gatewayRequest.Entities = append(gatewayRequest.Entities, gatewayclient.EntitySourceRef{SourceCode: entity.SourceCode, SourceEntityId: entity.SourceEntityId})
	}
	resolved := resolvedGatewayContext{EventsBySource: map[string]string{}, EntitiesBySource: map[string]string{}}
	if len(gatewayRequest.Events) > 0 || len(gatewayRequest.Entities) > 0 {
		resolved, err = s.resolveGatewayContext(ctx, scope, gatewayRequest)
		if err != nil {
			return nil, err
		}
	}
	input := model.ImportRequest{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(), Origin: "agent", Selection: resolved.Selection}
	for _, id := range request.Body.SomIssueIds {
		input.SomIssueIDs = append(input.SomIssueIDs, id.String())
	}
	for _, node := range request.Body.Nodes {
		converted := model.AgentNode{Ref: strings.TrimSpace(node.Ref)}
		if node.EventRef != nil {
			sourceKey, ok := eventRefs[strings.TrimSpace(*node.EventRef)]
			if !ok {
				return nil, validationError("node event_ref is not present in events")
			}
			v := resolved.EventsBySource[sourceKey]
			converted.SnapshotEventID = &v
		}
		if node.EntityRef != nil {
			sourceKey, ok := entityRefs[strings.TrimSpace(*node.EntityRef)]
			if !ok {
				return nil, validationError("node entity_ref is not present in entities")
			}
			v := resolved.EntitiesBySource[sourceKey]
			converted.SnapshotEntityID = &v
		}
		if node.NodeId != nil {
			v := node.NodeId.String()
			converted.NodeID = &v
		}
		input.Nodes = append(input.Nodes, converted)
	}
	for _, edge := range request.Body.Edges {
		input.Edges = append(input.Edges, model.AgentEdge{SourceRef: edge.SourceRef, TargetRef: edge.TargetRef, RelationCode: edge.RelationCode, Why: edge.Why, Confidence: edge.Confidence, EvidenceEventRefs: edge.EvidenceEventRefs})
	}
	stats, err := s.db.ImportContext(ctx, input)
	if err != nil {
		return nil, storeError(err)
	}
	return investigations.AddAgentResults201JSONResponse(importResult(stats)), nil
}

type resolvedGatewayContext struct {
	Selection        model.GatewaySelection
	EventsBySource   map[string]string
	EntitiesBySource map[string]string
}

func (s *Server) resolveGatewayContext(ctx context.Context, scope socctx.Scope, request gatewayclient.ResolveContextRequest) (resolvedGatewayContext, error) {
	if len(request.Events) == 0 && len(request.Entities) == 0 {
		return resolvedGatewayContext{}, validationError("at least one event or entity is required")
	}
	bearer, _ := socctx.BearerFromContext(ctx)
	response, err := s.gateway.ResolveContext(ctx, scope.ProjectID, bearer, request)
	if err != nil {
		return resolvedGatewayContext{}, gatewayError(err)
	}
	if len(response.SourceErrors) > 0 {
		for _, sourceErr := range response.SourceErrors {
			if sourceErr.Code == "source_record_not_found" {
				return resolvedGatewayContext{}, validationError("Gateway could not find a selected source record")
			}
		}
		return resolvedGatewayContext{}, httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, "Gateway could not resolve every selected source record")
	}
	resolved, err := convertGatewayContext(response)
	if err != nil {
		return resolvedGatewayContext{}, err
	}
	for _, ref := range request.Events {
		if _, ok := resolved.EventsBySource[sourceRecordKey(ref.SourceCode, ref.SourceEventId)]; !ok {
			return resolvedGatewayContext{}, validationError("Gateway did not return a selected event")
		}
	}
	for _, ref := range request.Entities {
		if _, ok := resolved.EntitiesBySource[sourceRecordKey(ref.SourceCode, ref.SourceEntityId)]; !ok {
			return resolvedGatewayContext{}, validationError("Gateway did not return a selected entity")
		}
	}
	return resolved, nil
}

func convertGatewayContext(input gatewayclient.ResolveContextResponse) (resolvedGatewayContext, error) {
	out := resolvedGatewayContext{EventsBySource: make(map[string]string), EntitiesBySource: make(map[string]string)}
	for _, entity := range input.Entities {
		snapshotID := entityKey(entity.Type, entity.Value)
		item := model.GatewayEntity{SnapshotID: snapshotID, TypeCode: entity.Type, Value: entity.Value, Attributes: entity.Attributes}
		for _, source := range entity.Sources {
			item.Provenance = append(item.Provenance, model.GatewayProvenance{Source: source.SourceCode, ExternalID: source.SourceEntityId, SourceURL: source.SourceRef, FetchedAt: source.FetchedAt})
			out.EntitiesBySource[sourceRecordKey(source.SourceCode, source.SourceEntityId)] = snapshotID
		}
		out.Selection.Entities = append(out.Selection.Entities, item)
	}
	for _, event := range input.Events {
		snapshotID := sourceRecordKey(event.SourceCode, event.SourceEventId)
		item := model.GatewayEvent{SnapshotID: snapshotID, Title: event.Title, EventType: event.Type, Severity: string(event.Severity), OccurredAt: event.OccurredAt, Attributes: event.Attributes, Provenance: model.GatewayProvenance{Source: event.SourceCode, ExternalID: event.SourceEventId, SourceURL: event.SourceRef, FetchedAt: event.FetchedAt}}
		for _, entity := range event.Entities {
			item.EntitySnapshotIDs = append(item.EntitySnapshotIDs, entityKey(entity.Type, entity.Value))
		}
		out.Selection.Events = append(out.Selection.Events, item)
		out.EventsBySource[sourceRecordKey(event.SourceCode, event.SourceEventId)] = snapshotID
	}
	for _, relation := range input.Relations {
		out.Selection.Relations = append(out.Selection.Relations, model.GatewayRelation{
			SnapshotID: sourceRecordKey(relation.SourceCode, relation.SourceRelationId), RelationCode: relation.Type,
			SourceEntitySnapshotID: entityKey(relation.SourceEntity.Type, relation.SourceEntity.Value),
			TargetEntitySnapshotID: entityKey(relation.TargetEntity.Type, relation.TargetEntity.Value),
			OccurredAt:             relation.OccurredAt,
			Provenance:             model.GatewayProvenance{Source: relation.SourceCode, ExternalID: relation.SourceRelationId, SourceURL: relation.SourceRef, FetchedAt: relation.FetchedAt},
		})
	}
	return out, nil
}

func sourceRecordKey(source, id string) string {
	return strings.TrimSpace(source) + "\x00" + strings.TrimSpace(id)
}

func entityKey(kind, value string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.TrimSpace(value)
}

func validationError(message string) error {
	return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, message)
}

func gatewayError(err error) error {
	var upstream *gatewayclient.HTTPError
	if errors.As(err, &upstream) && (upstream.Status == http.StatusBadRequest || upstream.Status == http.StatusNotFound || upstream.Status == http.StatusUnprocessableEntity) {
		return validationError("Gateway rejected one or more selected source records")
	}
	if errors.As(err, &upstream) && upstream.Status == http.StatusForbidden {
		return httperr.ErrForbidden
	}
	return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, "Gateway context resolver is unavailable")
}

func importResult(stats model.ImportStats) investigations.ContextImportResult {
	return investigations.ContextImportResult{Events: stats.Events, Entities: stats.Entities, Nodes: stats.Nodes, Edges: stats.Edges}
}

func convertInvestigation(inv model.Investigation) (investigations.Investigation, error) {
	id, err := dbUUID(inv.ID)
	if err != nil {
		return investigations.Investigation{}, err
	}
	origin := investigations.Origin(inv.Origin)
	out := investigations.Investigation{Id: id, ProjectId: inv.ProjectID, Title: inv.Title, Description: inv.Description, Status: investigations.InvestigationStatus(inv.Status), Severity: (*investigations.Severity)(inv.Severity), Verdict: (*investigations.Verdict)(inv.Verdict), VerdictReason: inv.VerdictReason, Confidence: inv.Confidence, Origin: &origin, OriginRef: inv.OriginRef, Version: inv.Version, SomWorkspaceIds: []uuid.UUID{}, CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt, ClosedAt: inv.ClosedAt}
	out.Counters.Children = inv.Counters.Children
	out.Counters.Events = inv.Counters.Events
	out.Counters.Entities = inv.Counters.Entities
	out.Counters.ProposedEdges = inv.Counters.ProposedEdges
	if inv.ParentID != nil {
		id, err := dbUUID(*inv.ParentID)
		if err != nil {
			return investigations.Investigation{}, err
		}
		out.ParentId = &id
	}
	for _, workspaceID := range inv.WorkspaceIDs {
		id, err := dbUUID(workspaceID)
		if err != nil {
			return investigations.Investigation{}, err
		}
		out.SomWorkspaceIds = append(out.SomWorkspaceIds, id)
	}
	return out, nil
}

func (s *Server) UpdateInvestigation(context.Context, investigations.UpdateInvestigationRequestObject) (investigations.UpdateInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) GetInvestigationTree(context.Context, investigations.GetInvestigationTreeRequestObject) (investigations.GetInvestigationTreeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteInvestigation(context.Context, investigations.DeleteInvestigationRequestObject) (investigations.DeleteInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
