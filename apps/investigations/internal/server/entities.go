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

// DeleteEntity Delete an entity
// (DELETE /entities/{entity_id})
func (s *Server) DeleteEntity(ctx context.Context, request entities.DeleteEntityRequestObject) (entities.DeleteEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// UpdateEntity Edit an entity
// (PATCH /entities/{entity_id})
func (s *Server) UpdateEntity(ctx context.Context, request entities.UpdateEntityRequestObject) (entities.UpdateEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListEntities Entities of the investigation
// (GET /investigations/{investigation_id}/entities)
func (s *Server) ListEntities(ctx context.Context, request entities.ListEntitiesRequestObject) (entities.ListEntitiesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// CreateEntity Add an entity by hand
// (POST /investigations/{investigation_id}/entities)
func (s *Server) CreateEntity(ctx context.Context, request entities.CreateEntityRequestObject) (entities.CreateEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DetachEntity Drop an entity from the investigation
// (DELETE /investigations/{investigation_id}/entities/{entity_id})
func (s *Server) DetachEntity(ctx context.Context, request entities.DetachEntityRequestObject) (entities.DetachEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
