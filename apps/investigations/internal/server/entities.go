package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/entities"
)

func (s *Server) CreateEntity(ctx context.Context, request entities.CreateEntityRequestObject) (entities.CreateEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteEntity(ctx context.Context, request entities.DeleteEntityRequestObject) (entities.DeleteEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetEntityCard(ctx context.Context, request entities.GetEntityCardRequestObject) (entities.GetEntityCardResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) UpdateEntity(ctx context.Context, request entities.UpdateEntityRequestObject) (entities.UpdateEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DetachEntity(ctx context.Context, request entities.DetachEntityRequestObject) (entities.DetachEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) ListEntities(ctx context.Context, request entities.ListEntitiesRequestObject) (entities.ListEntitiesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
