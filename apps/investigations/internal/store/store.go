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

	RoleBindings(ctx context.Context, subjectID, projectID string) (model.SubjectRoles, error)
}
