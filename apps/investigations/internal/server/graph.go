package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	coreauthctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/graph"
)

func (s *Server) GetGraph(ctx context.Context, request graph.GetGraphRequestObject) (graph.GetGraphResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	include := request.Params.IncludeSubtree != nil && *request.Params.IncludeSubtree
	statuses := statusStrings(request.Params.Statuses)
	nodes, err := s.db.GraphNodes(ctx, scope.ProjectID, request.InvestigationId.String(), model.NodeFilter{IncludeSubtree: include})
	if err != nil {
		return nil, storeError(err)
	}
	edges, err := s.db.GraphEdges(ctx, scope.ProjectID, request.InvestigationId.String(), model.EdgeFilter{IncludeSubtree: include, Statuses: statuses, MinConfidence: request.Params.MinConfidence})
	if err != nil {
		return nil, storeError(err)
	}
	out := graph.Graph{InvestigationId: request.InvestigationId, IncludeSubtree: &include, Nodes: make([]graph.GraphNode, 0, len(nodes)), Edges: make([]graph.GraphEdge, 0, len(edges))}
	for _, item := range nodes {
		converted, err := convertGraphNode(item)
		if err != nil {
			return nil, err
		}
		out.Nodes = append(out.Nodes, converted)
	}
	for _, item := range edges {
		converted, err := convertGraphEdge(item)
		if err != nil {
			return nil, err
		}
		out.Edges = append(out.Edges, converted)
	}
	return graph.GetGraph200JSONResponse(out), nil
}

func (s *Server) ListNodes(ctx context.Context, request graph.ListNodesRequestObject) (graph.ListNodesResponseObject, error) {
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
	filter := model.NodeFilter{Cursor: cursor, Limit: limit + 1}
	if request.Params.NodeType != nil {
		v := string(*request.Params.NodeType)
		filter.NodeType = &v
	}
	if request.Params.Q != nil {
		v := strings.TrimSpace(*request.Params.Q)
		if v != "" {
			filter.Q = &v
		}
	}
	items, err := s.db.GraphNodes(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := graph.NodePage{Nodes: make([]graph.GraphNode, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertGraphNode(item)
		if err != nil {
			return nil, err
		}
		page.Nodes = append(page.Nodes, converted)
	}
	return graph.ListNodes200JSONResponse(page), nil
}

func (s *Server) GetNode(ctx context.Context, request graph.GetNodeRequestObject) (graph.GetNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetNode(ctx, scope.ProjectID, request.InvestigationId.String(), request.NodeId.String())
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertGraphNode(item)
	if err != nil {
		return nil, err
	}
	return graph.GetNode200JSONResponse(out), nil
}

func (s *Server) ListGraphEdges(ctx context.Context, request graph.ListGraphEdgesRequestObject) (graph.ListGraphEdgesResponseObject, error) {
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
	filter := model.EdgeFilter{Statuses: statusStrings(request.Params.Statuses), RelationCode: request.Params.RelationCode, MinConfidence: request.Params.MinConfidence, Cursor: cursor, Limit: limit + 1}
	if request.Params.Origin != nil {
		v := string(*request.Params.Origin)
		filter.Origin = &v
	}
	if request.Params.NodeId != nil {
		v := request.Params.NodeId.String()
		filter.NodeID = &v
	}
	items, err := s.db.GraphEdges(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := graph.GraphEdgePage{Edges: make([]graph.GraphEdge, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertGraphEdge(item)
		if err != nil {
			return nil, err
		}
		page.Edges = append(page.Edges, converted)
	}
	return graph.ListGraphEdges200JSONResponse(page), nil
}

func statusStrings(values *[]graph.GraphEdgeStatus) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(*values))
	for _, value := range *values {
		out = append(out, string(value))
	}
	return out
}

func (s *Server) CreateNode(ctx context.Context, request graph.CreateNodeRequestObject) (graph.CreateNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	var entityID, eventID *string
	switch request.Body.NodeType {
	case graph.Entity:
		if request.Body.EntityId == nil || request.Body.EventId != nil {
			return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "entity node requires only entity_id")
		}
		v := request.Body.EntityId.String()
		entityID = &v
	case graph.Event:
		if request.Body.EventId == nil || request.Body.EntityId != nil {
			return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "event node requires only event_id")
		}
		v := request.Body.EventId.String()
		eventID = &v
	default:
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "invalid node_type")
	}
	var issues []string
	if request.Body.SomIssueIds != nil {
		for _, id := range *request.Body.SomIssueIds {
			issues = append(issues, id.String())
		}
	}
	node, err := s.db.CreateNode(ctx, scope.ProjectID, request.InvestigationId.String(), string(request.Body.NodeType), entityID, eventID, "analyst", issues)
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertGraphNode(node)
	if err != nil {
		return nil, err
	}
	return graph.CreateNode201JSONResponse(out), nil
}

func convertGraphNode(node model.GraphNode) (graph.GraphNode, error) {
	id, err := dbUUID(node.ID)
	if err != nil {
		return graph.GraphNode{}, err
	}
	investigationID, err := dbUUID(node.InvestigationID)
	if err != nil {
		return graph.GraphNode{}, err
	}
	out := graph.GraphNode{Id: id, InvestigationId: investigationID, NodeType: graph.NodeType(node.NodeType), Origin: graph.Origin(node.Origin), SomIssueIds: []uuid.UUID{}, Label: node.Label, TypeCode: node.TypeCode, CanonicalKey: node.CanonicalKey, OccurredAt: node.OccurredAt}
	if node.EntityID != nil {
		id, err := dbUUID(*node.EntityID)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.EntityId = &id
	}
	if node.EventID != nil {
		id, err := dbUUID(*node.EventID)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.EventId = &id
	}
	for _, value := range node.SomIssueIDs {
		id, err := dbUUID(value)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.SomIssueIds = append(out.SomIssueIds, id)
	}
	return out, nil
}

func convertGraphEdge(item model.GraphEdge) (graph.GraphEdge, error) {
	id, err := dbUUID(item.ID)
	if err != nil {
		return graph.GraphEdge{}, err
	}
	investigationID, err := dbUUID(item.InvestigationID)
	if err != nil {
		return graph.GraphEdge{}, err
	}
	sourceID, err := dbUUID(item.SourceNodeID)
	if err != nil {
		return graph.GraphEdge{}, err
	}
	targetID, err := dbUUID(item.TargetNodeID)
	if err != nil {
		return graph.GraphEdge{}, err
	}
	evidence := make([]openapi_types.UUID, 0, len(item.EvidenceEventIDs))
	for _, value := range item.EvidenceEventIDs {
		id, err := dbUUID(value)
		if err != nil {
			return graph.GraphEdge{}, err
		}
		evidence = append(evidence, id)
	}
	out := graph.GraphEdge{Id: id, InvestigationId: investigationID, SourceNodeId: sourceID, TargetNodeId: targetID, RelationCode: item.RelationCode, Status: graph.GraphEdgeStatus(item.Status), RejectReason: item.RejectReason, Confidence: item.Confidence, Why: item.Why, Origin: graph.Origin(item.Origin), OriginRef: item.OriginRef, EvidenceCount: len(evidence), EvidenceEventIds: &evidence, Version: item.Version, CreatedAt: &item.CreatedAt, UpdatedAt: &item.UpdatedAt}
	var metadata map[string]any
	if len(item.Metadata) > 0 {
		if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
			return graph.GraphEdge{}, err
		}
		out.Metadata = &metadata
	}
	return out, nil
}

func edgeStoreError(err error) error {
	var conflict *store.ConflictError
	if errors.As(err, &conflict) {
		return httperr.New(http.StatusConflict, httperr.CodeConflict,
			"operation conflicts with current graph state").WithDetails(map[string]any{"conflicts": conflict.IDs})
	}
	return storeError(err)
}

func (s *Server) CreateGraphEdge(ctx context.Context, request graph.CreateGraphEdgeRequestObject) (graph.CreateGraphEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	relationCode := strings.TrimSpace(request.Body.RelationCode)
	if relationCode == "" {
		return nil, validationError("relation_code is required")
	}
	if request.Body.Confidence != nil && (*request.Body.Confidence < 0 || *request.Body.Confidence > 1) {
		return nil, validationError("confidence must be between 0 and 1")
	}
	var why *string
	if request.Body.Why != nil {
		value := strings.TrimSpace(*request.Body.Why)
		if value != "" {
			why = &value
		}
	}
	var originRef *string
	if subjectID, ok := coreauthctx.SubjectIDFromContext(ctx); ok {
		originRef = &subjectID
	}
	input := model.GraphEdgeNew{
		ProjectID:       scope.ProjectID,
		InvestigationID: request.InvestigationId.String(),
		SourceNodeID:    request.Body.SourceNodeId.String(),
		TargetNodeID:    request.Body.TargetNodeId.String(),
		RelationCode:    relationCode,
		Confidence:      request.Body.Confidence,
		Why:             why,
		OriginRef:       originRef,
	}
	if request.Body.EvidenceEventIds != nil {
		for _, id := range *request.Body.EvidenceEventIds {
			input.EvidenceEventIDs = append(input.EvidenceEventIDs, id.String())
		}
	}
	edge, err := s.db.CreateGraphEdge(ctx, input)
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertGraphEdge(edge)
	if err != nil {
		return nil, err
	}
	return graph.CreateGraphEdge201JSONResponse(out), nil
}

func (s *Server) GetGraphEdge(ctx context.Context, request graph.GetGraphEdgeRequestObject) (graph.GetGraphEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	edge, err := s.db.GetGraphEdge(ctx, scope.ProjectID, request.InvestigationId.String(), request.EdgeId.String())
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertGraphEdge(edge)
	if err != nil {
		return nil, err
	}
	return graph.GetGraphEdge200JSONResponse(out), nil
}

func (s *Server) UpdateGraphEdge(ctx context.Context, request graph.UpdateGraphEdgeRequestObject) (graph.UpdateGraphEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if request.Body.Version < 1 {
		return nil, validationError("version must be positive")
	}
	if request.Body.Confidence != nil && (*request.Body.Confidence < 0 || *request.Body.Confidence > 1) {
		return nil, validationError("confidence must be between 0 and 1")
	}
	patch := model.GraphEdgePatch{
		ProjectID:       scope.ProjectID,
		InvestigationID: request.InvestigationId.String(),
		EdgeID:          request.EdgeId.String(),
		Version:         request.Body.Version,
		RejectReason:    request.Body.RejectReason,
		Confidence:      request.Body.Confidence,
		Why:             request.Body.Why,
	}
	if request.Body.Status != nil {
		value := string(*request.Body.Status)
		patch.Status = &value
	}
	if request.Body.Metadata != nil {
		patch.Metadata, _ = json.Marshal(*request.Body.Metadata)
		patch.HasMetadata = true
	}
	edge, err := s.db.UpdateGraphEdge(ctx, patch)
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertGraphEdge(edge)
	if err != nil {
		return nil, err
	}
	return graph.UpdateGraphEdge200JSONResponse(out), nil
}

func (s *Server) DeleteGraphEdge(ctx context.Context, request graph.DeleteGraphEdgeRequestObject) (graph.DeleteGraphEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteGraphEdge(ctx, scope.ProjectID, request.InvestigationId.String(), request.EdgeId.String()); err != nil {
		return nil, edgeStoreError(err)
	}
	return graph.DeleteGraphEdge204Response{}, nil
}

func convertEvidenceEvents(items []model.EvidenceEvent) ([]graph.EvidenceEvent, error) {
	out := make([]graph.EvidenceEvent, 0, len(items))
	for _, item := range items {
		id, err := dbUUID(item.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, graph.EvidenceEvent{EventId: id, SourceCode: item.SourceCode,
			SourceEventId: item.SourceEventID, SourceRef: item.SourceRef,
			EventType: item.EventType, OccurredAt: item.OccurredAt})
	}
	return out, nil
}

func (s *Server) ListGraphEdgeEvidence(ctx context.Context, request graph.ListGraphEdgeEvidenceRequestObject) (graph.ListGraphEdgeEvidenceResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.db.GraphEdgeEvidence(ctx, scope.ProjectID, request.InvestigationId.String(), request.EdgeId.String())
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertEvidenceEvents(items)
	if err != nil {
		return nil, err
	}
	return graph.ListGraphEdgeEvidence200JSONResponse(out), nil
}

func (s *Server) AddGraphEdgeEvidence(ctx context.Context, request graph.AddGraphEdgeEvidenceRequestObject) (graph.AddGraphEdgeEvidenceResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if len(request.Body.EventIds) == 0 {
		return nil, validationError("event_ids must not be empty")
	}
	ids := make([]string, 0, len(request.Body.EventIds))
	for _, id := range request.Body.EventIds {
		ids = append(ids, id.String())
	}
	items, err := s.db.AddGraphEdgeEvidence(ctx, scope.ProjectID, request.InvestigationId.String(), request.EdgeId.String(), ids)
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertEvidenceEvents(items)
	if err != nil {
		return nil, err
	}
	return graph.AddGraphEdgeEvidence200JSONResponse(out), nil
}

func (s *Server) DeleteGraphEdgeEvidence(ctx context.Context, request graph.DeleteGraphEdgeEvidenceRequestObject) (graph.DeleteGraphEdgeEvidenceResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteGraphEdgeEvidence(ctx, scope.ProjectID, request.InvestigationId.String(), request.EdgeId.String(), request.EventId.String()); err != nil {
		return nil, edgeStoreError(err)
	}
	return graph.DeleteGraphEdgeEvidence204Response{}, nil
}
func (s *Server) DeleteNode(context.Context, graph.DeleteNodeRequestObject) (graph.DeleteNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) ReviewGraphEdges(ctx context.Context, request graph.ReviewGraphEdgesRequestObject) (graph.ReviewGraphEdgesResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	input := model.EdgeReviewRequest{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String()}
	seen := make(map[string]struct{})
	if request.Body.Confirm != nil {
		for _, item := range *request.Body.Confirm {
			id := item.Id.String()
			if item.Version < 1 {
				return nil, validationError("review versions must be positive")
			}
			if _, duplicate := seen[id]; duplicate {
				return nil, validationError("review edge ids must be unique")
			}
			seen[id] = struct{}{}
			input.Confirm = append(input.Confirm, model.EdgeReviewItem{ID: id, Version: item.Version})
		}
	}
	if request.Body.Reject != nil {
		for _, item := range *request.Body.Reject {
			id := item.Id.String()
			reason := strings.TrimSpace(item.Reason)
			if item.Version < 1 || reason == "" {
				return nil, validationError("reject version and reason are required")
			}
			if _, duplicate := seen[id]; duplicate {
				return nil, validationError("review edge ids must be unique")
			}
			seen[id] = struct{}{}
			input.Reject = append(input.Reject, model.EdgeReviewItem{ID: id, Version: item.Version, Reason: &reason})
		}
	}
	if len(input.Confirm) == 0 && len(input.Reject) == 0 {
		return nil, validationError("at least one edge must be reviewed")
	}
	result, err := s.db.ReviewGraphEdges(ctx, input)
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out := graph.ReviewResult{Confirmed: make([]openapi_types.UUID, 0, len(result.Confirmed)), Rejected: make([]openapi_types.UUID, 0, len(result.Rejected))}
	for _, value := range result.Confirmed {
		id, err := dbUUID(value)
		if err != nil {
			return nil, err
		}
		out.Confirmed = append(out.Confirmed, id)
	}
	for _, value := range result.Rejected {
		id, err := dbUUID(value)
		if err != nil {
			return nil, err
		}
		out.Rejected = append(out.Rejected, id)
	}
	return graph.ReviewGraphEdges200JSONResponse(out), nil
}

// CreateHypothesisGraphEdge Draw a shared graph edge in a hypothesis
// (POST /investigations/{investigation_id}/hypotheses/{hypothesis_id}/edges)
func (s *Server) CreateHypothesisGraphEdge(ctx context.Context, request graph.CreateHypothesisGraphEdgeRequestObject) (graph.CreateHypothesisGraphEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	relationCode := strings.TrimSpace(request.Body.RelationCode)
	if relationCode == "" {
		return nil, validationError("relation_code is required")
	}
	if request.Body.Confidence != nil && (*request.Body.Confidence < 0 || *request.Body.Confidence > 1) {
		return nil, validationError("confidence must be between 0 and 1")
	}
	var why *string
	if request.Body.Why != nil {
		value := strings.TrimSpace(*request.Body.Why)
		if value != "" {
			why = &value
		}
	}
	var originRef *string
	if subjectID, ok := coreauthctx.SubjectIDFromContext(ctx); ok {
		originRef = &subjectID
	}
	input := model.GraphEdgeNew{
		ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(),
		SourceNodeID: request.Body.SourceNodeId.String(), TargetNodeID: request.Body.TargetNodeId.String(),
		RelationCode: relationCode, Confidence: request.Body.Confidence, Why: why, OriginRef: originRef,
	}
	if request.Body.EvidenceEventIds != nil {
		for _, id := range *request.Body.EvidenceEventIds {
			input.EvidenceEventIDs = append(input.EvidenceEventIDs, id.String())
		}
	}
	edge, err := s.db.CreateHypothesisGraphEdge(ctx, request.HypothesisId.String(), input)
	if err != nil {
		return nil, edgeStoreError(err)
	}
	out, err := convertGraphEdge(edge)
	if err != nil {
		return nil, err
	}
	return graph.CreateHypothesisGraphEdge201JSONResponse(out), nil
}

// GetHypothesisGraph Graph projection of a hypothesis
// (GET /investigations/{investigation_id}/hypotheses/{hypothesis_id}/graph)
func (s *Server) GetHypothesisGraph(ctx context.Context, request graph.GetHypothesisGraphRequestObject) (graph.GetHypothesisGraphResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Params.Statuses != nil {
		for _, status := range *request.Params.Statuses {
			if !status.Valid() {
				return nil, validationError("invalid graph edge status")
			}
		}
	}
	if request.Params.MinConfidence != nil && (*request.Params.MinConfidence < 0 || *request.Params.MinConfidence > 1) {
		return nil, validationError("min_confidence must be between 0 and 1")
	}
	projection, err := s.db.HypothesisGraph(ctx, scope.ProjectID, request.InvestigationId.String(),
		request.HypothesisId.String(), model.EdgeFilter{
			Statuses: statusStrings(request.Params.Statuses), MinConfidence: request.Params.MinConfidence,
		})
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	out := graph.HypothesisGraph{
		HypothesisId: request.HypothesisId, InvestigationId: request.InvestigationId,
		Nodes: make([]graph.GraphNode, 0, len(projection.Nodes)), Edges: make([]graph.GraphEdge, 0, len(projection.Edges)),
	}
	for _, item := range projection.Nodes {
		converted, err := convertGraphNode(item)
		if err != nil {
			return nil, err
		}
		out.Nodes = append(out.Nodes, converted)
	}
	for _, item := range projection.Edges {
		converted, err := convertGraphEdge(item)
		if err != nil {
			return nil, err
		}
		out.Edges = append(out.Edges, converted)
	}
	return graph.GetHypothesisGraph200JSONResponse(out), nil
}

// CreateHypothesisNode Create or find a graph node in a hypothesis
// (POST /investigations/{investigation_id}/hypotheses/{hypothesis_id}/nodes)
func (s *Server) CreateHypothesisNode(ctx context.Context, request graph.CreateHypothesisNodeRequestObject) (graph.CreateHypothesisNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	var entityID, eventID *string
	switch request.Body.NodeType {
	case graph.Entity:
		if request.Body.EntityId == nil || request.Body.EventId != nil {
			return nil, validationError("entity node requires only entity_id")
		}
		value := request.Body.EntityId.String()
		entityID = &value
	case graph.Event:
		if request.Body.EventId == nil || request.Body.EntityId != nil {
			return nil, validationError("event node requires only event_id")
		}
		value := request.Body.EventId.String()
		eventID = &value
	default:
		return nil, validationError("invalid node_type")
	}
	var issues []string
	if request.Body.SomIssueIds != nil {
		for _, id := range *request.Body.SomIssueIds {
			issues = append(issues, id.String())
		}
	}
	node, err := s.db.CreateHypothesisNode(ctx, scope.ProjectID, request.InvestigationId.String(),
		request.HypothesisId.String(), string(request.Body.NodeType), entityID, eventID, "analyst", issues)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	out, err := convertGraphNode(node)
	if err != nil {
		return nil, err
	}
	return graph.CreateHypothesisNode201JSONResponse(out), nil
}
