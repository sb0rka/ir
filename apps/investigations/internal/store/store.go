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

type Database interface {
	Ping(ctx context.Context) error
	Close()

	CreateInvestigation(ctx context.Context, inv model.InvestigationNew) (model.Investigation, error)
	ListInvestigations(ctx context.Context, projectID string, filter model.InvestigationFilter) ([]model.Investigation, error)
	GetInvestigation(ctx context.Context, projectID, investigationID string) (model.Investigation, error)
	InvestigationExists(ctx context.Context, projectID, investigationID string) (bool, error)
	ImportContext(ctx context.Context, request model.ImportRequest) (model.ImportStats, error)

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

	Reference(ctx context.Context) (model.Reference, error)
}
