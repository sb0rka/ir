package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
)

// AttachEvents Pull events into an investigation
// (POST /events)
func (s *Server) AttachEvents(ctx context.Context, request events.AttachEventsRequestObject) (events.AttachEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DeleteEvent Delete an event from the tenant
// (DELETE /events/{event_id})
func (s *Server) DeleteEvent(ctx context.Context, request events.DeleteEventRequestObject) (events.DeleteEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetEvent One event in full
// (GET /events/{event_id})
func (s *Server) GetEvent(ctx context.Context, request events.GetEventRequestObject) (events.GetEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DetachEvent Drop an event from the investigation
// (DELETE /events/{event_id}/investigations/{investigation_id})
func (s *Server) DetachEvent(ctx context.Context, request events.DetachEventRequestObject) (events.DetachEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// ListEvents Timeline of the investigation
// (GET /investigations/{investigation_id}/events)
func (s *Server) ListEvents(ctx context.Context, request events.ListEventsRequestObject) (events.ListEventsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
