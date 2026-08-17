// Package store — единственная дверь к базе. Новый запрос = метод интерфейса
// Database плюс реализация в psql.go; SQL пишется инлайном в методе.
package store

import (
	"context"
	"errors"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// Сентинелы для нарушений ссылочной целостности: транспорт превращает их
// в 404/422 не зная кодов Postgres.
var (
	ErrInvestigationNotFound = errors.New("investigation not found in this project")
	ErrParentNotFound        = errors.New("parent investigation not found in this project")
	ErrUnknownSource         = errors.New("source_code is not present in the sources dictionary")
	ErrTargetNotAttached     = errors.New("referenced entity or event is not attached to this investigation")
	ErrInvalidValue          = errors.New("value violates a domain constraint")
)

type Database interface {
	Ping(ctx context.Context) error
	Close()

	RoleBindings(ctx context.Context, subjectID, projectID string) (model.SubjectRoles, error)

	CreateInvestigation(ctx context.Context, inv model.InvestigationNew) (model.Investigation, error)
	ListInvestigations(ctx context.Context, projectID string, filter model.InvestigationFilter) ([]model.Investigation, error)
	InvestigationExists(ctx context.Context, projectID, investigationID string) (bool, error)

	// AttachEvents делает upsert событий и привязку к расследованию одной
	// транзакцией: частично засосанный pull хуже, чем несостоявшийся.
	AttachEvents(ctx context.Context, projectID, investigationID string,
		events []model.EventIngest, attachedBy string, reason *string) (model.AttachStats, error)

	// FindEventIDs резолвит refs в id существующих событий проекта.
	// Отсутствующие refs в карту не попадают — решает вызывающий.
	FindEventIDs(ctx context.Context, projectID string, refs []model.EventRef) (map[model.EventRef]string, error)

	// InvestigationEvents — таймлайн расследования. Курсорная пагинация не
	// реализована: отдаются первые Limit событий по occurred_at.
	InvestigationEvents(ctx context.Context, projectID, investigationID string,
		filter model.EventFilter) ([]model.EventSummary, error)

	// LinkEvents привязывает уже существующие события к расследованию.
	LinkEvents(ctx context.Context, projectID, investigationID string,
		eventIDs []string, attachedBy string, reason *string) (linked, duplicates int, err error)

	// CreateNode идемпотентен: повторная постановка той же сущности или
	// события на граф возвращает существующую ноду, дописывая som_issue связи.
	CreateNode(ctx context.Context, projectID, investigationID, nodeType string,
		entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error)

	GraphNodes(ctx context.Context, projectID, investigationID string, filter model.NodeFilter) ([]model.GraphNode, error)
}
