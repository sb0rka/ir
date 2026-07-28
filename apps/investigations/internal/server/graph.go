package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/graph"
)

// ListGraphEdges Edges of the investigation
// (GET /investigations/{investigation_id}/edges)
func (s *Server) ListGraphEdges(ctx context.Context, request graph.ListGraphEdgesRequestObject) (graph.ListGraphEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// CreateGraphEdge Draw an edge by hand
// (POST /investigations/{investigation_id}/edges)
func (s *Server) CreateGraphEdge(ctx context.Context, request graph.CreateGraphEdgeRequestObject) (graph.CreateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteGraphEdge Delete an edge
// (DELETE /investigations/{investigation_id}/edges/{edge_id})
func (s *Server) DeleteGraphEdge(ctx context.Context, request graph.DeleteGraphEdgeRequestObject) (graph.DeleteGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetGraphEdge One edge
// (GET /investigations/{investigation_id}/edges/{edge_id})
func (s *Server) GetGraphEdge(ctx context.Context, request graph.GetGraphEdgeRequestObject) (graph.GetGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// UpdateGraphEdge Edit one edge
// (PATCH /investigations/{investigation_id}/edges/{edge_id})
func (s *Server) UpdateGraphEdge(ctx context.Context, request graph.UpdateGraphEdgeRequestObject) (graph.UpdateGraphEdgeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListGraphEdgeEvidence What the edge rests on
// (GET /investigations/{investigation_id}/edges/{edge_id}/evidence)
func (s *Server) ListGraphEdgeEvidence(ctx context.Context, request graph.ListGraphEdgeEvidenceRequestObject) (graph.ListGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// AddGraphEdgeEvidence Cite more events
// (POST /investigations/{investigation_id}/edges/{edge_id}/evidence)
func (s *Server) AddGraphEdgeEvidence(ctx context.Context, request graph.AddGraphEdgeEvidenceRequestObject) (graph.AddGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteGraphEdgeEvidence Stop citing an event
// (DELETE /investigations/{investigation_id}/edges/{edge_id}/evidence/{event_id})
func (s *Server) DeleteGraphEdgeEvidence(ctx context.Context, request graph.DeleteGraphEdgeEvidenceRequestObject) (graph.DeleteGraphEdgeEvidenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetGraph Graph of the investigation
// (GET /investigations/{investigation_id}/graph)
func (s *Server) GetGraph(ctx context.Context, request graph.GetGraphRequestObject) (graph.GetGraphResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListNodes Nodes of the investigation
// (GET /investigations/{investigation_id}/nodes)
func (s *Server) ListNodes(ctx context.Context, request graph.ListNodesRequestObject) (graph.ListNodesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// CreateNode Put a node on the graph
// (POST /investigations/{investigation_id}/nodes)
func (s *Server) CreateNode(ctx context.Context, request graph.CreateNodeRequestObject) (graph.CreateNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteNode Take a node off the graph
// (DELETE /investigations/{investigation_id}/nodes/{node_id})
func (s *Server) DeleteNode(ctx context.Context, request graph.DeleteNodeRequestObject) (graph.DeleteNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetNode One node
// (GET /investigations/{investigation_id}/nodes/{node_id})
func (s *Server) GetNode(ctx context.Context, request graph.GetNodeRequestObject) (graph.GetNodeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ReviewGraphEdges Review proposed edges in bulk
// (POST /investigations/{investigation_id}/review)
func (s *Server) ReviewGraphEdges(ctx context.Context, request graph.ReviewGraphEdgesRequestObject) (graph.ReviewGraphEdgesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
