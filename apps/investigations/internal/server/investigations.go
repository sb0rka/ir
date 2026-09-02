package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
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
	for _, ref := range request.Body.Findings {
		converted, err := gatewaySourceObjectRef(ref)
		if err != nil {
			return nil, validationError("invalid finding source reference")
		}
		gatewayRequest.Findings = appendOptionalSlice(gatewayRequest.Findings, converted)
	}
	for _, ref := range request.Body.Sessions {
		converted, err := gatewaySourceObjectRef(ref)
		if err != nil {
			return nil, validationError("invalid session source reference")
		}
		gatewayRequest.Sessions = appendOptionalSlice(gatewayRequest.Sessions, converted)
	}
	for _, ref := range request.Body.Events {
		gatewayRequest.Events = appendOptionalSlice(gatewayRequest.Events, gatewayclient.EventSourceRef{SourceCode: ref.SourceCode, SourceEventId: ref.SourceEventId})
	}
	for _, ref := range request.Body.Entities {
		gatewayRequest.Entities = appendOptionalSlice(gatewayRequest.Entities, gatewayclient.EntitySourceRef{SourceCode: ref.SourceCode, SourceEntityId: ref.SourceEntityId})
	}
	resolved, err := s.resolveGatewayContext(ctx, scope, gatewayRequest)
	if err != nil {
		return nil, err
	}
	seed := request.Body.Seed != nil && *request.Body.Seed
	stats, err := s.db.ImportContext(ctx, model.ImportRequest{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(), Selection: resolved.Selection, Origin: "analyst", Warnings: resolved.Warnings, Seed: seed})
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
		gatewayRequest.Events = appendOptionalSlice(gatewayRequest.Events, gatewayclient.EventSourceRef{SourceCode: event.SourceCode, SourceEventId: event.SourceEventId})
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
		gatewayRequest.Entities = appendOptionalSlice(gatewayRequest.Entities, gatewayclient.EntitySourceRef{SourceCode: entity.SourceCode, SourceEntityId: entity.SourceEntityId})
	}
	resolved := resolvedGatewayContext{EventsBySource: map[string]string{}, EntitiesBySource: map[string]string{}}
	if len(optionalSlice(gatewayRequest.Events)) > 0 || len(optionalSlice(gatewayRequest.Entities)) > 0 {
		resolved, err = s.resolveGatewayContext(ctx, scope, gatewayRequest)
		if err != nil {
			return nil, err
		}
	}
	input := model.ImportRequest{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(), Origin: "agent", Selection: resolved.Selection, Warnings: resolved.Warnings}
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
		if node.EventId != nil {
			v := node.EventId.String()
			converted.EventID = &v
		}
		if node.EntityId != nil {
			v := node.EntityId.String()
			converted.EntityID = &v
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
	FindingsBySource map[string]string
	SessionsBySource map[string]string
	EventsBySource   map[string]string
	EntitiesBySource map[string]string
	Warnings         []string
}

func (s *Server) resolveGatewayContext(ctx context.Context, scope socctx.Scope, request gatewayclient.ResolveContextRequest) (resolvedGatewayContext, error) {
	findings := optionalSlice(request.Findings)
	sessions := optionalSlice(request.Sessions)
	events := optionalSlice(request.Events)
	entities := optionalSlice(request.Entities)
	if len(findings) == 0 && len(sessions) == 0 && len(events) == 0 && len(entities) == 0 {
		return resolvedGatewayContext{}, validationError("at least one finding, session, event, or entity is required")
	}
	bearer, _ := socctx.BearerFromContext(ctx)
	response, err := s.gateway.ResolveContext(ctx, scope.ProjectID, bearer, request)
	if err != nil {
		return resolvedGatewayContext{}, gatewayError(err)
	}
	resolved, err := convertGatewayContext(response, request)
	if err != nil {
		return resolvedGatewayContext{}, err
	}
	selectedReturned := 0
	for _, ref := range findings {
		key := sourceObjectKey(modelSourceObjectRef(ref))
		if _, ok := resolved.FindingsBySource[key]; ok {
			selectedReturned++
		} else {
			resolved.Warnings = append(resolved.Warnings, "Gateway did not return selected finding "+ref.ExternalId)
		}
	}
	for _, ref := range sessions {
		key := sourceObjectKey(modelSourceObjectRef(ref))
		if _, ok := resolved.SessionsBySource[key]; ok {
			selectedReturned++
		} else {
			resolved.Warnings = append(resolved.Warnings, "Gateway did not return selected session "+ref.ExternalId)
		}
	}
	for _, ref := range events {
		if _, ok := resolved.EventsBySource[sourceRecordKey(ref.SourceCode, ref.SourceEventId)]; !ok {
			resolved.Warnings = append(resolved.Warnings, "Gateway did not return selected event "+ref.SourceEventId)
		} else {
			selectedReturned++
		}
	}
	for _, ref := range entities {
		if _, ok := resolved.EntitiesBySource[sourceRecordKey(ref.SourceCode, ref.SourceEntityId)]; !ok {
			resolved.Warnings = append(resolved.Warnings, "Gateway did not return selected entity "+ref.SourceEntityId)
		} else {
			selectedReturned++
		}
	}
	if selectedReturned == 0 {
		return resolvedGatewayContext{}, validationError("Gateway did not return any selected source record")
	}
	for _, sourceErr := range response.SourceErrors {
		resolved.Warnings = append(resolved.Warnings, "Gateway source "+sourceErr.Source+": "+sourceErr.Message)
	}
	return resolved, nil
}

func convertGatewayContext(input gatewayclient.ResolveContextResponse, request gatewayclient.ResolveContextRequest) (resolvedGatewayContext, error) {
	findings := optionalSlice(request.Findings)
	sessions := optionalSlice(request.Sessions)
	events := optionalSlice(request.Events)
	entities := optionalSlice(request.Entities)
	out := resolvedGatewayContext{
		FindingsBySource: make(map[string]string), SessionsBySource: make(map[string]string),
		EventsBySource: make(map[string]string), EntitiesBySource: make(map[string]string),
	}
	selectedFindings := make(map[string]struct{}, len(findings))
	selectedSessions := make(map[string]struct{}, len(sessions))
	selectedEvents := make(map[string]struct{}, len(events))
	selectedEntities := make(map[string]struct{}, len(entities))
	for _, ref := range findings {
		selectedFindings[sourceObjectKey(modelSourceObjectRef(ref))] = struct{}{}
	}
	for _, ref := range sessions {
		selectedSessions[sourceObjectKey(modelSourceObjectRef(ref))] = struct{}{}
	}
	for _, ref := range events {
		selectedEvents[sourceRecordKey(ref.SourceCode, ref.SourceEventId)] = struct{}{}
	}
	for _, ref := range entities {
		selectedEntities[sourceRecordKey(ref.SourceCode, ref.SourceEntityId)] = struct{}{}
	}
	type resolution struct {
		status string
		errors []model.GatewayContextError
	}
	resolutions := make(map[string]resolution, len(input.Resolutions))
	for _, item := range input.Resolutions {
		converted := resolution{status: string(item.Status)}
		for _, sourceErr := range item.Errors {
			converted.errors = append(converted.errors, model.GatewayContextError{Source: sourceErr.Source, Code: sourceErr.Code, Message: sourceErr.Message, Retryable: sourceErr.Retryable})
		}
		resolutions[sourceObjectKey(modelSourceObjectRef(item.Ref))] = converted
	}
	for _, entity := range input.Entities {
		snapshotID := entityKey(entity.Type, entity.Value)
		item := model.GatewayEntity{SnapshotID: snapshotID, TypeCode: entity.Type, Value: entity.Value, Attributes: entity.Attributes}
		for _, source := range entity.Sources {
			item.Provenance = append(item.Provenance, model.GatewayProvenance{Source: source.SourceCode, ExternalID: source.SourceEntityId, SourceURL: source.SourceRef, FetchedAt: source.FetchedAt})
			key := sourceRecordKey(source.SourceCode, source.SourceEntityId)
			out.EntitiesBySource[key] = snapshotID
			if _, ok := selectedEntities[key]; ok {
				item.Direct = true
			}
		}
		out.Selection.Entities = append(out.Selection.Entities, item)
	}
	for _, event := range input.Events {
		snapshotID := sourceRecordKey(event.SourceCode, event.SourceEventId)
		_, direct := selectedEvents[snapshotID]
		item := model.GatewayEvent{SnapshotID: snapshotID, Direct: direct, Title: event.Title, EventType: event.Type, Severity: string(event.Severity), OccurredAt: event.OccurredAt, Attributes: event.Attributes, Provenance: model.GatewayProvenance{Source: event.SourceCode, ExternalID: event.SourceEventId, SourceURL: event.SourceRef, FetchedAt: event.FetchedAt}}
		for _, entity := range event.Entities {
			mention := model.GatewayEventEntity{SnapshotID: entityKey(entity.Type, entity.Value)}
			for _, role := range entity.Roles {
				mention.Roles = append(mention.Roles, string(role))
			}
			item.Entities = append(item.Entities, mention)
		}
		out.Selection.Events = append(out.Selection.Events, item)
		out.EventsBySource[sourceRecordKey(event.SourceCode, event.SourceEventId)] = snapshotID
	}
	for _, finding := range input.Findings {
		ref := modelSourceObjectRef(finding.Ref)
		key := sourceObjectKey(ref)
		_, direct := selectedFindings[key]
		contextStatus := "complete"
		var contextErrors []model.GatewayContextError
		if state, ok := resolutions[key]; ok {
			contextStatus, contextErrors = state.status, state.errors
		}
		normalized, err := json.Marshal(finding)
		if err != nil {
			return resolvedGatewayContext{}, err
		}
		provenance, _ := json.Marshal(map[string]any{"ref": ref, "source_ref": finding.SourceRef, "fetched_at": finding.FetchedAt})
		item := model.GatewayFinding{
			SnapshotID: key, Ref: ref, Kind: string(finding.Kind), Title: finding.Title,
			Description: finding.Description, Severity: string(finding.Severity), OccurredAt: finding.OccurredAt,
			Status: finding.Status, SourceRef: finding.SourceRef, FetchedAt: finding.FetchedAt,
			Normalized: normalized, Provenance: provenance, ContextStatus: contextStatus,
			ContextErrors: contextErrors, Direct: direct,
		}
		for _, entity := range finding.Entities {
			item.EntitySnapshotIDs = append(item.EntitySnapshotIDs, entityKey(entity.Type, entity.Value))
		}
		if finding.RelatedFindings != nil {
			for _, related := range *finding.RelatedFindings {
				item.RelatedFindings = append(item.RelatedFindings, modelSourceObjectRef(related))
			}
		}
		if finding.RelatedSessions != nil {
			for _, related := range *finding.RelatedSessions {
				item.RelatedSessions = append(item.RelatedSessions, modelSourceObjectRef(related))
			}
		}
		out.Selection.Findings = append(out.Selection.Findings, item)
		out.FindingsBySource[key] = key
	}
	for _, session := range input.Sessions {
		ref := modelSourceObjectRef(session.Ref)
		key := sourceObjectKey(ref)
		_, direct := selectedSessions[key]
		contextStatus := "complete"
		var contextErrors []model.GatewayContextError
		if state, ok := resolutions[key]; ok {
			contextStatus, contextErrors = state.status, state.errors
		}
		normalized, err := json.Marshal(session)
		if err != nil {
			return resolvedGatewayContext{}, err
		}
		provenance, _ := json.Marshal(map[string]any{"ref": ref, "source_ref": session.SourceRef, "fetched_at": session.FetchedAt})
		item := model.GatewaySession{
			SnapshotID: key, Ref: ref, Title: session.Title, Severity: string(session.Severity),
			StartedAt: session.StartedAt, EndedAt: session.EndedAt, SourceRef: session.SourceRef,
			FetchedAt: session.FetchedAt, Normalized: normalized, Provenance: provenance,
			ContextStatus: contextStatus, ContextErrors: contextErrors, Direct: direct,
		}
		for _, entity := range session.Entities {
			item.EntitySnapshotIDs = append(item.EntitySnapshotIDs, entityKey(entity.Type, entity.Value))
		}
		for _, related := range session.RelatedFindings {
			item.RelatedFindings = append(item.RelatedFindings, modelSourceObjectRef(related))
		}
		out.Selection.Sessions = append(out.Selection.Sessions, item)
		out.SessionsBySource[key] = key
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
	assignGatewayObjectOwnership(&out.Selection)
	return out, nil
}

// assignGatewayObjectOwnership keeps detach ownership bounded to context that
// can be attributed from the normalized Gateway response. It intentionally
// leaves unmarked vendor events unowned instead of assigning them to every
// coarse object returned by a multi-root resolve.
func assignGatewayObjectOwnership(selection *model.GatewaySelection) {
	for index := range selection.Findings {
		finding := &selection.Findings[index]
		refs := []model.GatewayObjectRef{finding.Ref}
		if finding.Ref.RecordType == "siem_incident" {
			refs = append(refs, finding.RelatedFindings...)
		}
		finding.EventSnapshotIDs = ownedEventSnapshotIDs(selection.Events, refs)
		finding.EntitySnapshotIDs = ownedEntitySnapshotIDs(finding.EntitySnapshotIDs, selection.Events, finding.EventSnapshotIDs)
	}
	for index := range selection.Sessions {
		session := &selection.Sessions[index]
		session.EventSnapshotIDs = ownedEventSnapshotIDs(selection.Events, []model.GatewayObjectRef{session.Ref})
		session.EntitySnapshotIDs = ownedEntitySnapshotIDs(session.EntitySnapshotIDs, selection.Events, session.EventSnapshotIDs)
	}
}

func ownedEventSnapshotIDs(events []model.GatewayEvent, refs []model.GatewayObjectRef) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, event := range events {
		for _, ref := range refs {
			if !eventBelongsToObject(event, ref) {
				continue
			}
			if _, duplicate := seen[event.SnapshotID]; !duplicate {
				seen[event.SnapshotID] = struct{}{}
				result = append(result, event.SnapshotID)
			}
			break
		}
	}
	return result
}

func ownedEntitySnapshotIDs(explicit []string, events []model.GatewayEvent, ownedEventIDs []string) []string {
	result := make([]string, 0, len(explicit))
	seen := make(map[string]struct{}, len(explicit))
	appendID := func(snapshotID string) {
		if _, duplicate := seen[snapshotID]; duplicate {
			return
		}
		seen[snapshotID] = struct{}{}
		result = append(result, snapshotID)
	}
	for _, snapshotID := range explicit {
		appendID(snapshotID)
	}
	ownedEvents := make(map[string]struct{}, len(ownedEventIDs))
	for _, snapshotID := range ownedEventIDs {
		ownedEvents[snapshotID] = struct{}{}
	}
	for _, event := range events {
		if _, owned := ownedEvents[event.SnapshotID]; !owned {
			continue
		}
		for _, mention := range event.Entities {
			appendID(mention.SnapshotID)
		}
	}
	return result
}

func eventBelongsToObject(event model.GatewayEvent, ref model.GatewayObjectRef) bool {
	if event.Provenance.Source != ref.SourceCode {
		return false
	}
	eventID := strings.TrimSpace(event.Provenance.ExternalID)
	if ref.SourceInstance != "" && !strings.HasPrefix(eventID, ref.SourceInstance+":") {
		return false
	}
	switch ref.RecordType {
	case "siem_incident":
		parentID, _ := event.Attributes["parent_finding_id"].(string)
		return strings.TrimSpace(parentID) == ref.ExternalID
	case "siem_correlation":
		if eventID == ref.ExternalID {
			return true
		}
		parentID, _ := event.Attributes["parent_source_event_id"].(string)
		relationType, _ := event.Attributes["relation_type"].(string)
		return relationType == "subevent_of" && strings.TrimSpace(parentID) == ref.ExternalID
	case "nad_attack":
		return eventID == ref.SourceInstance+":"+ref.RecordType+":"+ref.ExternalID
	case "nad_session":
		if eventID == ref.SourceInstance+":"+ref.RecordType+":"+ref.ExternalID {
			return true
		}
		parentID, _ := event.Attributes["parent_session_id"].(string)
		return strings.TrimSpace(parentID) == ref.ExternalID
	default:
		return false
	}
}

func gatewaySourceObjectRef(input any) (gatewayclient.SourceObjectRef, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return gatewayclient.SourceObjectRef{}, err
	}
	var out gatewayclient.SourceObjectRef
	if err := json.Unmarshal(raw, &out); err != nil {
		return gatewayclient.SourceObjectRef{}, err
	}
	if !out.TimeRange.From.Before(out.TimeRange.To) {
		return gatewayclient.SourceObjectRef{}, errors.New("invalid time range")
	}
	return out, nil
}

func modelSourceObjectRef(ref gatewayclient.SourceObjectRef) model.GatewayObjectRef {
	sourceInstance := ""
	if ref.SourceInstance != nil {
		sourceInstance = *ref.SourceInstance
	}
	return model.GatewayObjectRef{
		SourceCode: strings.TrimSpace(ref.SourceCode), SourceInstance: strings.TrimSpace(sourceInstance), RecordType: strings.TrimSpace(string(ref.RecordType)),
		ExternalID: strings.TrimSpace(ref.ExternalId), TimeRange: model.GatewayTimeRange{From: ref.TimeRange.From, To: ref.TimeRange.To},
	}
}

func sourceObjectKey(ref model.GatewayObjectRef) string {
	return strings.Join([]string{strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceInstance), strings.TrimSpace(ref.RecordType), strings.TrimSpace(ref.ExternalID)}, "\x00")
}

func sourceRecordKey(source, id string) string {
	return strings.TrimSpace(source) + "\x00" + strings.TrimSpace(id)
}

func entityKey(kind, value string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.TrimSpace(value)
}

func optionalSlice[T any](items *[]T) []T {
	if items == nil {
		return nil
	}
	return *items
}

func appendOptionalSlice[T any](items *[]T, value T) *[]T {
	if items == nil {
		slice := []T{value}
		return &slice
	}
	*items = append(*items, value)
	return items
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
	return investigations.ContextImportResult{Findings: stats.Findings, Sessions: stats.Sessions, Events: stats.Events, Entities: stats.Entities, Nodes: stats.Nodes, Edges: stats.Edges, Warnings: append([]string{}, stats.Warnings...)}
}

func convertInvestigation(inv model.Investigation) (investigations.Investigation, error) {
	id, err := dbUUID(inv.ID)
	if err != nil {
		return investigations.Investigation{}, err
	}
	origin := investigations.Origin(inv.Origin)
	out := investigations.Investigation{Id: id, ProjectId: inv.ProjectID, Title: inv.Title, Description: inv.Description, Status: investigations.InvestigationStatus(inv.Status), Severity: (*investigations.Severity)(inv.Severity), Verdict: (*investigations.Verdict)(inv.Verdict), VerdictReason: inv.VerdictReason, Confidence: inv.Confidence, Origin: &origin, OriginRef: inv.OriginRef, Version: inv.Version, SomWorkspaceIds: []uuid.UUID{}, CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt, ClosedAt: inv.ClosedAt}
	out.Counters.Children = inv.Counters.Children
	out.Counters.Findings = inv.Counters.Findings
	out.Counters.Sessions = inv.Counters.Sessions
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

func (s *Server) UpdateInvestigation(ctx context.Context, request investigations.UpdateInvestigationRequestObject) (investigations.UpdateInvestigationResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	patch := model.InvestigationPatch{
		ProjectID:       scope.ProjectID,
		InvestigationID: request.InvestigationId.String(),
		Version:         request.Body.Version,
		Title:           request.Body.Title,
		Description:     request.Body.Description,
		Confidence:      request.Body.Confidence,
		VerdictReason:   request.Body.VerdictReason,
	}
	if request.Body.Status != nil {
		value := string(*request.Body.Status)
		patch.Status = &value
	}
	if request.Body.Verdict != nil {
		value := string(*request.Body.Verdict)
		patch.Verdict = &value
	}
	if request.Body.Severity != nil {
		value := string(*request.Body.Severity)
		patch.Severity = &value
	}
	if request.Body.SomWorkspaceIds != nil {
		values := make([]string, 0, len(*request.Body.SomWorkspaceIds))
		for _, id := range *request.Body.SomWorkspaceIds {
			values = append(values, id.String())
		}
		patch.WorkspaceIDs = &values
	}
	updated, err := s.db.UpdateInvestigation(ctx, patch)
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertInvestigation(updated)
	if err != nil {
		return nil, err
	}
	return investigations.UpdateInvestigation200JSONResponse(out), nil
}
func (s *Server) GetInvestigationTree(context.Context, investigations.GetInvestigationTreeRequestObject) (investigations.GetInvestigationTreeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteInvestigation(ctx context.Context, request investigations.DeleteInvestigationRequestObject) (investigations.DeleteInvestigationResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteInvestigation(ctx, scope.ProjectID, request.InvestigationId.String()); err != nil {
		return nil, storeError(err)
	}
	return investigations.DeleteInvestigation204Response{}, nil
}

// AddHypothesisAgentResults Save explicit SOM agent results for an active hypothesis
// (POST /investigations/{investigation_id}/hypotheses/{hypothesis_id}/agent-results)
func (s *Server) AddHypothesisAgentResults(ctx context.Context, request investigations.AddHypothesisAgentResultsRequestObject) (investigations.AddHypothesisAgentResultsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if len(request.Body.SomIssueIds) == 0 {
		return nil, validationError("som_issue_ids must not be empty")
	}
	investigationID := request.InvestigationId.String()
	hypothesisID := request.HypothesisId.String()
	hypothesis, err := s.db.GetHypothesis(ctx, scope.ProjectID, investigationID, hypothesisID)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	if hypothesis.Status != "active" {
		return nil, hypothesisStoreError(&store.ConflictError{IDs: []string{hypothesisID}})
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
		gatewayRequest.Events = appendOptionalSlice(gatewayRequest.Events,
			gatewayclient.EventSourceRef{SourceCode: event.SourceCode, SourceEventId: event.SourceEventId})
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
		gatewayRequest.Entities = appendOptionalSlice(gatewayRequest.Entities,
			gatewayclient.EntitySourceRef{SourceCode: entity.SourceCode, SourceEntityId: entity.SourceEntityId})
	}
	resolved := resolvedGatewayContext{EventsBySource: map[string]string{}, EntitiesBySource: map[string]string{}}
	if len(optionalSlice(gatewayRequest.Events)) > 0 || len(optionalSlice(gatewayRequest.Entities)) > 0 {
		resolved, err = s.resolveGatewayContext(ctx, scope, gatewayRequest)
		if err != nil {
			return nil, err
		}
	}
	input := model.ImportRequest{
		ProjectID: scope.ProjectID, InvestigationID: investigationID,
		HypothesisID: &hypothesisID, RequireActiveHypothesis: true,
		Origin: "agent", Selection: resolved.Selection, Warnings: resolved.Warnings,
	}
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
			value := resolved.EventsBySource[sourceKey]
			converted.SnapshotEventID = &value
		}
		if node.EntityRef != nil {
			sourceKey, ok := entityRefs[strings.TrimSpace(*node.EntityRef)]
			if !ok {
				return nil, validationError("node entity_ref is not present in entities")
			}
			value := resolved.EntitiesBySource[sourceKey]
			converted.SnapshotEntityID = &value
		}
		if node.EventId != nil {
			value := node.EventId.String()
			converted.EventID = &value
		}
		if node.EntityId != nil {
			value := node.EntityId.String()
			converted.EntityID = &value
		}
		if node.NodeId != nil {
			value := node.NodeId.String()
			converted.NodeID = &value
		}
		input.Nodes = append(input.Nodes, converted)
	}
	for _, edge := range request.Body.Edges {
		input.Edges = append(input.Edges, model.AgentEdge{
			SourceRef: edge.SourceRef, TargetRef: edge.TargetRef, RelationCode: edge.RelationCode,
			Why: edge.Why, Confidence: edge.Confidence, EvidenceEventRefs: edge.EvidenceEventRefs,
		})
	}
	stats, err := s.db.ImportContext(ctx, input)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	return investigations.AddHypothesisAgentResults201JSONResponse(importResult(stats)), nil
}

// AddHypothesisContext Add analyst-selected context through a hypothesis
// (POST /investigations/{investigation_id}/hypotheses/{hypothesis_id}/context)
func (s *Server) AddHypothesisContext(ctx context.Context, request investigations.AddHypothesisContextRequestObject) (investigations.AddHypothesisContextResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	investigationID := request.InvestigationId.String()
	hypothesisID := request.HypothesisId.String()
	hypothesis, err := s.db.GetHypothesis(ctx, scope.ProjectID, investigationID, hypothesisID)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	if hypothesis.Status == "resolved" {
		return nil, hypothesisStoreError(&store.ConflictError{IDs: []string{hypothesisID}})
	}
	gatewayRequest := gatewayclient.ResolveContextRequest{}
	for _, ref := range request.Body.Findings {
		converted, err := gatewaySourceObjectRef(ref)
		if err != nil {
			return nil, validationError("invalid finding source reference")
		}
		gatewayRequest.Findings = appendOptionalSlice(gatewayRequest.Findings, converted)
	}
	for _, ref := range request.Body.Sessions {
		converted, err := gatewaySourceObjectRef(ref)
		if err != nil {
			return nil, validationError("invalid session source reference")
		}
		gatewayRequest.Sessions = appendOptionalSlice(gatewayRequest.Sessions, converted)
	}
	for _, ref := range request.Body.Events {
		gatewayRequest.Events = appendOptionalSlice(gatewayRequest.Events,
			gatewayclient.EventSourceRef{SourceCode: ref.SourceCode, SourceEventId: ref.SourceEventId})
	}
	for _, ref := range request.Body.Entities {
		gatewayRequest.Entities = appendOptionalSlice(gatewayRequest.Entities,
			gatewayclient.EntitySourceRef{SourceCode: ref.SourceCode, SourceEntityId: ref.SourceEntityId})
	}
	resolved, err := s.resolveGatewayContext(ctx, scope, gatewayRequest)
	if err != nil {
		return nil, err
	}
	stats, err := s.db.ImportContext(ctx, model.ImportRequest{
		ProjectID: scope.ProjectID, InvestigationID: investigationID, HypothesisID: &hypothesisID,
		Selection: resolved.Selection, Origin: "analyst", Warnings: resolved.Warnings,
	})
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	return investigations.AddHypothesisContext201JSONResponse(importResult(stats)), nil
}
