package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/entities"
)

// Карточка сущности
// GET /entities/{entity_id}
func (s *Server) GetEntityCard(
	_ context.Context, _ entities.GetEntityCardRequestObject,
) (entities.GetEntityCardResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
