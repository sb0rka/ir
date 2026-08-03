package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
)

func (s *Server) AttachEvents(ctx context.Context, request events.AttachEventsRequestObject) (events.AttachEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteEvent(ctx context.Context, request events.DeleteEventRequestObject) (events.DeleteEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetEvent(ctx context.Context, request events.GetEventRequestObject) (events.GetEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DetachEvent(ctx context.Context, request events.DetachEventRequestObject) (events.DetachEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) ListEvents(ctx context.Context, request events.ListEventsRequestObject) (events.ListEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
