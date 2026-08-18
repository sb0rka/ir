package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
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

func (s *Server) CreateGraphEdge(context.Context, graph.CreateGraphEdgeRequestObject) (graph.CreateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteGraphEdge(context.Context, graph.DeleteGraphEdgeRequestObject) (graph.DeleteGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) GetGraphEdge(context.Context, graph.GetGraphEdgeRequestObject) (graph.GetGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) UpdateGraphEdge(context.Context, graph.UpdateGraphEdgeRequestObject) (graph.UpdateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) ListGraphEdgeEvidence(context.Context, graph.ListGraphEdgeEvidenceRequestObject) (graph.ListGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) AddGraphEdgeEvidence(context.Context, graph.AddGraphEdgeEvidenceRequestObject) (graph.AddGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteGraphEdgeEvidence(context.Context, graph.DeleteGraphEdgeEvidenceRequestObject) (graph.DeleteGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteNode(context.Context, graph.DeleteNodeRequestObject) (graph.DeleteNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) ReviewGraphEdges(context.Context, graph.ReviewGraphEdgesRequestObject) (graph.ReviewGraphEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
