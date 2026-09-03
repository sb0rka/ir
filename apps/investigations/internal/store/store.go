// Package store defines the project-scoped persistence boundary.
package store

import (
	"context"
	"errors"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

var (
	ErrInvestigationNotFound = errors.New("investigation not found in this project")
	ErrParentNotFound        = errors.New("parent investigation not found in this project")
	ErrRecordNotFound        = errors.New("record not found in this project")
	ErrTargetNotAttached     = errors.New("referenced target is not attached to this investigation")
	ErrUnknownReference      = errors.New("unknown local or gateway reference")
	ErrConflict              = errors.New("operation conflicts with confirmed graph evidence")
	ErrInvalidValue          = errors.New("value violates a domain constraint")
)

type ConflictError struct {
	IDs []string
}

func (e *ConflictError) Error() string { return "operation conflicts with current graph state" }
func (e *ConflictError) Unwrap() error { return ErrConflict }

type Database interface {
	Ping(ctx context.Context) error
	Close()

	CreateInvestigation(ctx context.Context, inv model.InvestigationNew) (model.Investigation, error)
	ListInvestigations(ctx context.Context, projectID string, filter model.InvestigationFilter) ([]model.Investigation, error)
	GetInvestigation(ctx context.Context, projectID, investigationID string) (model.Investigation, error)
	UpdateInvestigation(ctx context.Context, patch model.InvestigationPatch) (model.Investigation, error)
	DeleteInvestigation(ctx context.Context, projectID, investigationID string) error
	InvestigationExists(ctx context.Context, projectID, investigationID string) (bool, error)
	ImportContext(ctx context.Context, request model.ImportRequest) (model.ImportStats, error)

	CreateHypothesis(ctx context.Context, hypothesis model.HypothesisNew) (model.Hypothesis, error)
	ListHypotheses(ctx context.Context, projectID, investigationID string, filter model.HypothesisFilter) ([]model.Hypothesis, error)
	GetHypothesis(ctx context.Context, projectID, investigationID, hypothesisID string) (model.Hypothesis, error)
	UpdateHypothesis(ctx context.Context, patch model.HypothesisPatch) (model.Hypothesis, error)
	DeleteHypothesis(ctx context.Context, projectID, investigationID, hypothesisID string) error
	HypothesisGraph(ctx context.Context, projectID, investigationID, hypothesisID string, filter model.EdgeFilter) (model.HypothesisGraph, error)
	AddHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeID string) error
	DeleteHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeID string) error
	AddHypothesisEdge(ctx context.Context, projectID, investigationID, hypothesisID, edgeID string) error
	DeleteHypothesisEdge(ctx context.Context, projectID, investigationID, hypothesisID, edgeID string) error
	CreateHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeType string, entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error)
	CreateHypothesisGraphEdge(ctx context.Context, hypothesisID string, edge model.GraphEdgeNew) (model.GraphEdge, error)

	InvestigationFindings(ctx context.Context, projectID, investigationID string, filter model.ObjectFilter) ([]model.Finding, error)
	GetFinding(ctx context.Context, projectID, findingID string) (model.Finding, error)
	DetachFinding(ctx context.Context, projectID, investigationID, findingID string) error

	InvestigationSessions(ctx context.Context, projectID, investigationID string, filter model.ObjectFilter) ([]model.NetworkSession, error)
	GetSession(ctx context.Context, projectID, sessionID string) (model.NetworkSession, error)
	DetachSession(ctx context.Context, projectID, investigationID, sessionID string) error

	InvestigationEvents(ctx context.Context, projectID, investigationID string, filter model.EventFilter) ([]model.EventSummary, error)
	GetEvent(ctx context.Context, projectID, eventID string) (model.Event, error)
	DetachEvent(ctx context.Context, projectID, investigationID, eventID string) error

	InvestigationEntities(ctx context.Context, projectID, investigationID string, filter model.EntityFilter) ([]model.Entity, error)
	GetEntityCard(ctx context.Context, projectID, entityID string) (model.EntityCard, error)
	CreateEntity(ctx context.Context, entity model.EntityNew) (model.Entity, error)
	DetachEntity(ctx context.Context, projectID, investigationID, entityID string) error

	CreateNode(ctx context.Context, projectID, investigationID, nodeType string, entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error)
	GraphNodes(ctx context.Context, projectID, investigationID string, filter model.NodeFilter) ([]model.GraphNode, error)
	GetNode(ctx context.Context, projectID, investigationID, nodeID string) (model.GraphNode, error)
	GraphEdges(ctx context.Context, projectID, investigationID string, filter model.EdgeFilter) ([]model.GraphEdge, error)
	CreateGraphEdge(ctx context.Context, edge model.GraphEdgeNew) (model.GraphEdge, error)
	GetGraphEdge(ctx context.Context, projectID, investigationID, edgeID string) (model.GraphEdge, error)
	UpdateGraphEdge(ctx context.Context, patch model.GraphEdgePatch) (model.GraphEdge, error)
	DeleteGraphEdge(ctx context.Context, projectID, investigationID, edgeID string) error
	GraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID string) ([]model.EvidenceEvent, error)
	AddGraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID string, eventIDs []string) ([]model.EvidenceEvent, error)
	DeleteGraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID, eventID string) error
	ReviewGraphEdges(ctx context.Context, request model.EdgeReviewRequest) (model.EdgeReviewResult, error)

	Reference(ctx context.Context) (model.Reference, error)
}
