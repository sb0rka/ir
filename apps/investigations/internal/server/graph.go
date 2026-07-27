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
