// Package server implements the generated API contracts.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

type Server struct {
	db  store.Database
	log *slog.Logger

	// Демо-интеграции. som ходит в SOM/relay/daemon с токеном вызывающего,
	// gateway резолвит attachEvents.query, prompt — адреса для агента.
	som     *somclient.Client
	gateway *gatewayclient.Client
	prompt  config.PromptConfig
}

var _ transport.API = (*Server)(nil)

func New(db store.Database, log *slog.Logger,
	som *somclient.Client, gateway *gatewayclient.Client, prompt config.PromptConfig) *Server {

	return &Server{db: db, log: log, som: som, gateway: gateway, prompt: prompt}
}

func (s *Server) Resolve(ctx context.Context, subjectID, projectID string) (transport.Roles, error) {
	roles, err := s.db.RoleBindings(ctx, subjectID, projectID)
	if err != nil {
		return transport.Roles{}, err
	}
	return transport.Roles{ProjectID: roles.ProjectID, Roles: roles.Roles}, nil
}

var _ transport.RoleResolver = (*Server)(nil)

// Хелперы живут здесь, а не в отдельном файле: gen-prune удаляет из пакета
// любой файл, не названный по домену из api/investigations/paths.

// scope — project scope запроса. Доменные ручки без него не работают:
// project_id участвует в каждом запросе к базе.
func (s *Server) scope(ctx context.Context) (socctx.Scope, error) {
	scope, ok := socctx.ScopeFromContext(ctx)
	if !ok {
		return socctx.Scope{}, httperr.BadRequest("X-Project-ID header is required")
	}
	return scope, nil
}

// storeError переводит сентинелы стора в HTTP-коды контракта.
func storeError(err error) error {
	switch {
	case errors.Is(err, store.ErrInvestigationNotFound), errors.Is(err, store.ErrParentNotFound):
		return httperr.ErrNotFound
	case errors.Is(err, store.ErrUnknownSource):
		return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"source_code is not present in the sources dictionary")
	case errors.Is(err, store.ErrTargetNotAttached):
		return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"referenced entity or event is not attached to this investigation")
	case errors.Is(err, store.ErrInvalidValue):
		return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"value violates a domain constraint")
	}
	return err
}

// dbUUID парсит id, пришедший из базы. Ошибка здесь — повреждённые данные,
// а не вина клиента.
func dbUUID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, httperr.New(http.StatusInternalServerError, httperr.CodeInternal,
			"storage returned a malformed identifier")
	}
	return parsed, nil
}
