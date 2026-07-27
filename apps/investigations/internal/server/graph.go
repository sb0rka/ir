package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/graph"
)

// Создать ребро (аналитик)
// POST /investigations/{investigation_id}/edges
func (s *Server) CreateEdge(
	_ context.Context, _ graph.CreateEdgeRequestObject,
) (graph.CreateEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Граф расследования
// GET /investigations/{investigation_id}/graph
func (s *Server) GetGraph(
	_ context.Context, _ graph.GetGraphRequestObject,
) (graph.GetGraphResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Батч-ревизия proposed-рёбер
// POST /investigations/{investigation_id}/review
func (s *Server) ReviewEdges(
	_ context.Context, _ graph.ReviewEdgesRequestObject,
) (graph.ReviewEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// CreateNode Put a node on the graph
// (POST /investigations/{investigation_id}/nodes)
func (s *Server) CreateNode(ctx context.Context, request graph.CreateNodeRequestObject) (graph.CreateNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteEdge Delete an edge
// (DELETE /edges/{edge_id})
func (s *Server) DeleteEdge(ctx context.Context, request graph.DeleteEdgeRequestObject) (graph.DeleteEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetEdge One edge
// (GET /edges/{edge_id})
func (s *Server) GetEdge(ctx context.Context, request graph.GetEdgeRequestObject) (graph.GetEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// UpdateEdge Edit one edge
// (PATCH /edges/{edge_id})
func (s *Server) UpdateEdge(ctx context.Context, request graph.UpdateEdgeRequestObject) (graph.UpdateEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListEdgeEvidence What the edge rests on
// (GET /edges/{edge_id}/evidence)
func (s *Server) ListEdgeEvidence(ctx context.Context, request graph.ListEdgeEvidenceRequestObject) (graph.ListEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// AddEdgeEvidence Cite more events
// (POST /edges/{edge_id}/evidence)
func (s *Server) AddEdgeEvidence(ctx context.Context, request graph.AddEdgeEvidenceRequestObject) (graph.AddEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteEdgeEvidence Stop citing an event
// (DELETE /edges/{edge_id}/evidence/{event_id})
func (s *Server) DeleteEdgeEvidence(ctx context.Context, request graph.DeleteEdgeEvidenceRequestObject) (graph.DeleteEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListEdges Edges of the investigation
// (GET /investigations/{investigation_id}/edges)
func (s *Server) ListEdges(ctx context.Context, request graph.ListEdgesRequestObject) (graph.ListEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListNodes Nodes of the investigation
// (GET /investigations/{investigation_id}/nodes)
func (s *Server) ListNodes(ctx context.Context, request graph.ListNodesRequestObject) (graph.ListNodesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteNode Take a node off the graph
// (DELETE /nodes/{node_id})
func (s *Server) DeleteNode(ctx context.Context, request graph.DeleteNodeRequestObject) (graph.DeleteNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetNode One node
// (GET /nodes/{node_id})
func (s *Server) GetNode(ctx context.Context, request graph.GetNodeRequestObject) (graph.GetNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
