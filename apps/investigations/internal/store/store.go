// Package store — единственная дверь к базе. Новый запрос = метод интерфейса
// Database плюс реализация в psql.go; SQL пишется инлайном в методе.
package store

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

type Database interface {
	Ping(ctx context.Context) error
	Close()

	// RoleBindings отдаёт тенант и роли субъекта — на них стоит вся авторизация.
	RoleBindings(ctx context.Context, subjectID string) (model.SubjectRoles, error)

	ListSources(ctx context.Context) ([]model.Source, error)
	ListEntityTypes(ctx context.Context) ([]model.EntityType, error)
	ListRelationTypes(ctx context.Context) ([]model.RelationType, error)
}
