package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
)

// Событие целиком
// GET /events/{event_id}
func (s *Server) GetEvent(
	_ context.Context, _ events.GetEventRequestObject,
) (events.GetEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// События расследования (таймлайн)
// GET /investigations/{investigation_id}/events
func (s *Server) ListEvents(
	_ context.Context, _ events.ListEventsRequestObject,
) (events.ListEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Затянуть события в расследование
// POST /investigations/{investigation_id}/events
func (s *Server) AttachEvents(
	_ context.Context, _ events.AttachEventsRequestObject,
) (events.AttachEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteEvent Drop an event from the investigation
// (DELETE /events/{event_id})
func (s *Server) DeleteEvent(ctx context.Context, request events.DeleteEventRequestObject) (events.DeleteEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
