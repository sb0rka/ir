package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
)

func (s *Server) ListEvents(ctx context.Context, request events.ListEventsRequestObject) (events.ListEventsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "limit must be between 1 and 200")
	}
	cursor, err := decodeCursor(request.Params.Cursor)
	if err != nil {
		return nil, err
	}
	filter := model.EventFilter{EventType: request.Params.EventType, SourceCode: request.Params.SourceCode, From: request.Params.From, To: request.Params.To, Cursor: cursor, Limit: limit + 1}
	if request.Params.EntityId != nil {
		v := request.Params.EntityId.String()
		filter.EntityID = &v
	}
	if request.Params.Q != nil {
		v := strings.TrimSpace(*request.Params.Q)
		if v != "" {
			filter.Q = &v
		}
	}
	items, err := s.db.InvestigationEvents(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := events.EventPage{Events: make([]events.EventSummary, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.OccurredAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertEventSummary(item)
		if err != nil {
			return nil, err
		}
		page.Events = append(page.Events, converted)
	}
	return events.ListEvents200JSONResponse(page), nil
}

func (s *Server) GetEvent(ctx context.Context, request events.GetEventRequestObject) (events.GetEventResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetEvent(ctx, scope.ProjectID, request.EventId.String())
	if err != nil {
		return nil, storeError(err)
	}
	id, err := dbUUID(item.ID)
	if err != nil {
		return nil, err
	}
	out := events.Event{Id: id, SourceCode: item.SourceCode, SourceEventId: item.SourceEventID, SourceRef: item.SourceRef, Title: item.Title, EventType: item.EventType, OccurredAt: item.OccurredAt, IngestedAt: item.IngestedAt}
	var normalized map[string]any
	if err := json.Unmarshal(item.NormalizedData, &normalized); err != nil {
		return nil, err
	}
	out.NormalizedData = &normalized
	ids := make([]openapi_types.UUID, 0, len(item.InvestigationIDs))
	for _, value := range item.InvestigationIDs {
		id, err := dbUUID(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out.InvestigationIds = &ids
	relations := make([]struct {
		EntityId     openapi_types.UUID `json:"entity_id"`
		RelationCode string             `json:"relation_code"`
	}, 0, len(item.Entities))
	for _, relation := range item.Entities {
		id, err := dbUUID(relation.EntityID)
		if err != nil {
			return nil, err
		}
		relations = append(relations, struct {
			EntityId     openapi_types.UUID `json:"entity_id"`
			RelationCode string             `json:"relation_code"`
		}{id, relation.RelationCode})
	}
	out.Entities = &relations
	return events.GetEvent200JSONResponse(out), nil
}

func convertEventSummary(item model.EventSummary) (events.EventSummary, error) {
	id, err := dbUUID(item.ID)
	if err != nil {
		return events.EventSummary{}, err
	}
	actor := events.Actor(item.AttachedBy)
	out := events.EventSummary{Id: id, SourceCode: item.SourceCode, SourceEventId: item.SourceEventID, SourceRef: item.SourceRef, Title: item.Title, EventType: item.EventType, OccurredAt: item.OccurredAt, IngestedAt: item.IngestedAt, AttachedAt: &item.AttachedAt, AttachedBy: &actor, Reason: item.Reason}
	var normalized map[string]any
	if err := json.Unmarshal(item.NormalizedData, &normalized); err != nil {
		return events.EventSummary{}, err
	}
	out.NormalizedData = &normalized
	return out, nil
}

func (s *Server) DetachEvent(ctx context.Context, request events.DetachEventRequestObject) (events.DetachEventResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DetachEvent(ctx, scope.ProjectID, request.InvestigationId.String(), request.EventId.String()); err != nil {
		return nil, storeError(err)
	}
	return events.DetachEvent204Response{}, nil
}
func (s *Server) DeleteEvent(context.Context, events.DeleteEventRequestObject) (events.DeleteEventResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
