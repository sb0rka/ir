package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/triage"
)

// Лента сработок источников
// GET /alerts
func (s *Server) ListAlerts(
	_ context.Context, _ triage.ListAlertsRequestObject,
) (triage.ListAlertsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Сквозной поиск по источникам
// GET /search
func (s *Server) Search(
	_ context.Context, _ triage.SearchRequestObject,
) (triage.SearchResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
