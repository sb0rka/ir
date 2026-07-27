// Package server реализует контракт: файл на домен, метод на ручку.
// Заготовки дописывает `task gen` через josharian/impl — они возвращают 501,
// пока не написана реализация.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
)

type Server struct {
	db  store.Database
	log *slog.Logger
}

var _ transport.API = (*Server)(nil)

func New(db store.Database, log *slog.Logger) *Server {
	return &Server{db: db, log: log}
}

// Resolve — реализация transport.RoleResolver: тенант и роли берутся из БД
// по субъекту токена, а не из тела запроса.
func (s *Server) Resolve(ctx context.Context, subjectID string) (transport.Roles, error) {
	roles, err := s.db.RoleBindings(ctx, subjectID)
	if err != nil {
		// Роли в нескольких тенантах — состояние данных, а не сбой сервера.
		// Без этой ветки клиент получал бы 500 и не понимал, что чинить.
		if errors.Is(err, store.ErrAmbiguousTenant) {
			return transport.Roles{}, httperr.New(http.StatusConflict, httperr.CodeConflict,
				"subject has roles in several projects; tenant is ambiguous")
		}
		return transport.Roles{}, err
	}
	return transport.Roles{ProjectID: roles.ProjectID, Roles: roles.Roles}, nil
}

var _ transport.RoleResolver = (*Server)(nil)
