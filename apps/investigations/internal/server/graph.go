package server

import (
	"context"

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

func (s *Server) GetGraph(ctx context.Context, request graph.GetGraphRequestObject) (graph.GetGraphResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) ListNodes(ctx context.Context, request graph.ListNodesRequestObject) (graph.ListNodesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) CreateNode(ctx context.Context, request graph.CreateNodeRequestObject) (graph.CreateNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
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
