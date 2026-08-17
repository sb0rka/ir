package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/graph"
)

func (s *Server) ListGraphEdges(ctx context.Context, request graph.ListGraphEdgesRequestObject) (graph.ListGraphEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) CreateGraphEdge(ctx context.Context, request graph.CreateGraphEdgeRequestObject) (graph.CreateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteGraphEdge(ctx context.Context, request graph.DeleteGraphEdgeRequestObject) (graph.DeleteGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetGraphEdge(ctx context.Context, request graph.GetGraphEdgeRequestObject) (graph.GetGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) UpdateGraphEdge(ctx context.Context, request graph.UpdateGraphEdgeRequestObject) (graph.UpdateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) ListGraphEdgeEvidence(ctx context.Context, request graph.ListGraphEdgeEvidenceRequestObject) (graph.ListGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) AddGraphEdgeEvidence(ctx context.Context, request graph.AddGraphEdgeEvidenceRequestObject) (graph.AddGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteGraphEdgeEvidence(ctx context.Context, request graph.DeleteGraphEdgeEvidenceRequestObject) (graph.DeleteGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetGraph отдаёт ноды с их som_issue связями — read-only проверка результата
// демо. Edges в скоупе не реализованы и всегда пустые.
func (s *Server) GetGraph(ctx context.Context, request graph.GetGraphRequestObject) (graph.GetGraphResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}

	nodes, err := s.db.GraphNodes(ctx, scope.ProjectID, request.InvestigationId.String(), model.NodeFilter{})
	if err != nil {
		return nil, storeError(err)
	}

	out := graph.Graph{
		InvestigationId: request.InvestigationId,
		Nodes:           make([]graph.GraphNode, 0, len(nodes)),
		Edges:           []graph.GraphEdge{},
	}
	for _, node := range nodes {
		converted, err := convertGraphNode(node)
		if err != nil {
			return nil, err
		}
		out.Nodes = append(out.Nodes, converted)
	}
	return graph.GetGraph200JSONResponse(out), nil
}

func (s *Server) ListNodes(ctx context.Context, request graph.ListNodesRequestObject) (graph.ListNodesResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Params.Cursor != nil {
		return nil, httperr.ErrNotImplemented
	}

	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"limit must be between 1 and 200")
	}

	filter := model.NodeFilter{Limit: limit}
	if request.Params.NodeType != nil {
		nodeType := string(*request.Params.NodeType)
		filter.NodeType = &nodeType
	}
	if request.Params.Q != nil {
		q := *request.Params.Q
		if q != "" {
			filter.Q = &q
		}
	}

	nodes, err := s.db.GraphNodes(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}

	page := graph.NodePage{Items: make([]graph.GraphNode, 0, len(nodes))}
	for _, node := range nodes {
		converted, err := convertGraphNode(node)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, converted)
	}
	return graph.ListNodes200JSONResponse(page), nil
}

// CreateNode ставит на граф сущность или событие. Идемпотентен: повторный
// вызов возвращает существующую ноду, дописав som_issue связи, — агент может
// безопасно ретраить.
func (s *Server) CreateNode(ctx context.Context, request graph.CreateNodeRequestObject) (graph.CreateNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	body := request.Body

	var entityID, eventID *string
	switch body.NodeType {
	case graph.Entity:
		if body.EntityId == nil || body.EventId != nil {
			return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
				"node_type entity requires entity_id and forbids event_id")
		}
		value := body.EntityId.String()
		entityID = &value
	case graph.Event:
		if body.EventId == nil || body.EntityId != nil {
			return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
				"node_type event requires event_id and forbids entity_id")
		}
		value := body.EventId.String()
		eventID = &value
	default:
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"node_type must be entity or event")
	}

	var somIssueIDs []string
	if body.SomIssueIds != nil {
		for _, issueID := range *body.SomIssueIds {
			somIssueIDs = append(somIssueIDs, issueID.String())
		}
	}
	// Ссылка на SOM issue — след запуска агентом; постановка ноды руками
	// через API остаётся за аналитиком.
	origin := string(graph.Analyst)
	if len(somIssueIDs) > 0 {
		origin = string(graph.Agent)
	}

	node, err := s.db.CreateNode(ctx, scope.ProjectID, request.InvestigationId.String(),
		string(body.NodeType), entityID, eventID, origin, somIssueIDs)
	if err != nil {
		return nil, storeError(err)
	}
	converted, err := convertGraphNode(node)
	if err != nil {
		return nil, err
	}
	return graph.CreateNode201JSONResponse(converted), nil
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

	out := graph.GraphNode{
		Id:              id,
		InvestigationId: investigationID,
		NodeType:        graph.NodeType(node.NodeType),
		Origin:          graph.Origin(node.Origin),
		SomIssueIds:     []uuid.UUID{},
		Label:           node.Label,
		OccurredAt:      node.OccurredAt,
	}
	if node.EntityID != nil {
		entityID, err := dbUUID(*node.EntityID)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.EntityId = &entityID
	}
	if node.EventID != nil {
		eventID, err := dbUUID(*node.EventID)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.EventId = &eventID
	}
	for _, issueID := range node.SomIssueIDs {
		parsed, err := dbUUID(issueID)
		if err != nil {
			return graph.GraphNode{}, err
		}
		out.SomIssueIds = append(out.SomIssueIds, parsed)
	}
	return out, nil
}

func (s *Server) DeleteNode(ctx context.Context, request graph.DeleteNodeRequestObject) (graph.DeleteNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetNode(ctx context.Context, request graph.GetNodeRequestObject) (graph.GetNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) ReviewGraphEdges(ctx context.Context, request graph.ReviewGraphEdgesRequestObject) (graph.ReviewGraphEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
