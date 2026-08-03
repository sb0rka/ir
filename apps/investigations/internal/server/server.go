// Package server реализует контракт: файл на домен, метод на ручку.
// Заготовки дописывает `task gen` через josharian/impl — они возвращают 501,
// пока не написана реализация.
package server

import (
	"context"
	"log/slog"

	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
)

type Server struct {
	db  store.Database
	log *slog.Logger
}

var _ transport.API = (*Server)(nil)

func New(db store.Database, log *slog.Logger) *Server {
	return &Server{db: db, log: log}
}

// Resolve возвращает только роли субъекта в явно выбранном проекте.
func (s *Server) Resolve(ctx context.Context, subjectID, projectID string) (transport.Roles, error) {
	roles, err := s.db.RoleBindings(ctx, subjectID, projectID)
	if err != nil {
		return transport.Roles{}, err
	}
	return transport.Roles{ProjectID: roles.ProjectID, Roles: roles.Roles}, nil
}

var _ transport.RoleResolver = (*Server)(nil)
