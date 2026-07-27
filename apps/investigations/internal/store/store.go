// Package store — единственная дверь к базе. Новый запрос = метод интерфейса
// Database плюс реализация в psql.go; SQL пишется инлайном в методе.
package store

import (
	"context"
	"errors"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// ErrAmbiguousTenant — у субъекта роли в нескольких проектах, а запрос не
// говорит, в каком он действует. Живёт здесь, а не в psql: транспорту нужно
// отличать это состояние данных от сбоя базы, не зная про драйвер.
var ErrAmbiguousTenant = errors.New("subject has role bindings in multiple projects")

type Database interface {
	Ping(ctx context.Context) error
	Close()

	// RoleBindings отдаёт тенант и роли субъекта — на них стоит вся авторизация.
	RoleBindings(ctx context.Context, subjectID string) (model.SubjectRoles, error)

	ListSources(ctx context.Context) ([]model.Source, error)
	ListEntityTypes(ctx context.Context) ([]model.EntityType, error)
	ListRelationTypes(ctx context.Context) ([]model.RelationType, error)
}
