package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/events"
)

// Записи, которые агент насосал за один pull, помечаются как его: attachEvents
// — рабочая ручка фазы «агент насыщает граф», аналитик события руками
// не приносит.
const attachedByAgent = "agent"

// Gateway не отдаёт больше 100 событий за страницу — остальное добирается
// курсором.
const gatewayPageLimit = 100

// Канонические типы сущностей, под которыми пробуется entity_key.
// Покрывают и типы mock-сценария (host/ip/process/file), и типы из контракта
// gateway (hostname/domain/hash/email).
var entityPivotKinds = []string{
	"host", "hostname", "ip", "domain", "process", "file", "hash", "email", "account", "url",
}

func (s *Server) AttachEvents(ctx context.Context, request events.AttachEventsRequestObject) (events.AttachEventsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	body := request.Body
	if (body.Query == nil) == (body.Refs == nil) {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"exactly one of query and refs must be set")
	}
	investigationID := body.InvestigationId.String()

	if body.Refs != nil {
		return s.attachByRefs(ctx, scope.ProjectID, investigationID, body)
	}
	return s.attachByQuery(ctx, scope.ProjectID, investigationID, body)
}

// attachByQuery резолвит выборку через gateway и записывает найденное:
// upsert в events + привязка к расследованию одной транзакцией.
func (s *Server) attachByQuery(ctx context.Context, projectID, investigationID string,
	body *events.EventAttachRequest) (events.AttachEventsResponseObject, error) {

	if !s.gateway.Configured() {
		return nil, httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable,
			"gateway integration is not configured: GATEWAY_BASE_URL is not set")
	}
	query := body.Query

	limit := 100
	if query.Limit != nil {
		limit = *query.Limit
	}
	if limit < 1 || limit > 500 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"limit must be between 1 and 500")
	}

	search := gatewayclient.SearchRequest{Sources: []string{query.SourceCode}}
	// entity_key — пивот по сущности. Тип сущности в запросе attachEvents не
	// указывается, поэтому значение отправляется под всеми каноническими
	// типами: событие матчится при точном совпадении хотя бы одного из них.
	if query.EntityKey != nil && strings.TrimSpace(*query.EntityKey) != "" {
		key := strings.TrimSpace(*query.EntityKey)
		for _, kind := range entityPivotKinds {
			search.Entities = append(search.Entities, gatewayclient.EntityRef{Type: kind, Value: key})
		}
	}
	// substring уходит свободным текстом — источник интерпретирует его сам
	// (mock MaxPatrol матчит по подстроке в title/class/severity).
	if query.Substring != nil && strings.TrimSpace(*query.Substring) != "" {
		search.Query = strings.TrimSpace(*query.Substring)
	}
	// TimeRange gateway требует обе границы — окно передается только целиком.
	if query.From != nil && query.To != nil {
		search.TimeRange = &gatewayclient.TimeRange{From: *query.From, To: *query.To}
	}

	found, err := s.collectGatewayEvents(ctx, projectID, search, limit)
	if err != nil {
		return nil, err
	}

	ingest := make([]model.EventIngest, 0, len(found))
	for _, event := range found {
		converted, err := convertGatewayEvent(query.SourceCode, event)
		if err != nil {
			return nil, err
		}
		ingest = append(ingest, converted)
	}

	stats, err := s.db.AttachEvents(ctx, projectID, investigationID, ingest, attachedByAgent, body.Reason)
	if err != nil {
		return nil, storeError(err)
	}
	return attachResult(stats), nil
}

// attachByRefs привязывает события, уже известные проекту. Gateway не умеет
// адресовать записи по id, поэтому ref, которого нет в базе, — ошибка
// с перечнем отсутствующих, а не тихий пропуск.
func (s *Server) attachByRefs(ctx context.Context, projectID, investigationID string,
	body *events.EventAttachRequest) (events.AttachEventsResponseObject, error) {

	refs := make([]model.EventRef, 0, len(*body.Refs))
	for _, ref := range *body.Refs {
		refs = append(refs, model.EventRef{
			SourceCode:    ref.SourceCode,
			SourceEventID: ref.SourceEventId,
		})
	}
	if len(refs) == 0 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"refs must not be empty")
	}

	found, err := s.db.FindEventIDs(ctx, projectID, refs)
	if err != nil {
		return nil, storeError(err)
	}

	var missing []string
	eventIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		id, ok := found[ref]
		if !ok {
			missing = append(missing, ref.SourceCode+"/"+ref.SourceEventID)
			continue
		}
		eventIDs = append(eventIDs, id)
	}
	if len(missing) > 0 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"refs address events unknown to this project — ingest them via query first").
			WithDetails(map[string]any{"missing": missing})
	}

	linked, duplicates, err := s.db.LinkEvents(ctx, projectID, investigationID, eventIDs, attachedByAgent, body.Reason)
	if err != nil {
		return nil, storeError(err)
	}
	return attachResult(model.AttachStats{Reused: linked, Duplicates: duplicates}), nil
}

func (s *Server) collectGatewayEvents(ctx context.Context, projectID string,
	search gatewayclient.SearchRequest, limit int) ([]gatewayclient.Event, error) {

	var collected []gatewayclient.Event
	for len(collected) < limit {
		search.Limit = min(limit-len(collected), gatewayPageLimit)
		page, err := s.gateway.SearchEvents(ctx, projectID, search)
		if err != nil {
			return nil, gatewayError(err)
		}
		collected = append(collected, page.Events...)
		if page.NextCursor == nil || *page.NextCursor == "" || len(page.Events) == 0 {
			break
		}
		search.Cursor = *page.NextCursor
	}
	if len(collected) > limit {
		collected = collected[:limit]
	}
	return collected, nil
}

func convertGatewayEvent(sourceCode string, event gatewayclient.Event) (model.EventIngest, error) {
	normalized, err := json.Marshal(map[string]any{
		"title":      event.Title,
		"severity":   event.Severity,
		"type":       event.Type,
		"attributes": event.Attributes,
		"entity_ids": event.EntityIDs,
	})
	if err != nil {
		return model.EventIngest{}, fmt.Errorf("encode normalized event: %w", err)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return model.EventIngest{}, fmt.Errorf("encode raw event: %w", err)
	}
	return model.EventIngest{
		SourceCode:     sourceCode,
		SourceEventID:  event.Provenance.ExternalID,
		SourceRef:      event.Provenance.SourceURL,
		EventType:      event.Type,
		OccurredAt:     event.OccurredAt,
		NormalizedData: normalized,
		RawData:        raw,
		DedupKey:       sourceCode + ":" + event.Provenance.ExternalID,
	}, nil
}

func attachResult(stats model.AttachStats) events.AttachEvents201JSONResponse {
	reused := stats.Reused
	return events.AttachEvents201JSONResponse(events.EventAttachResult{
		Attached:   stats.Attached,
		Reused:     &reused,
		Duplicates: stats.Duplicates,
		// Извлечение сущностей и связей из normalized_data — вне скоупа демо.
		EntitiesExtracted: 0,
		EdgesCreated:      0,
	})
}

func gatewayError(err error) error {
	var upstream *gatewayclient.UpstreamError
	if errors.As(err, &upstream) {
		return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, upstream.Error())
	}
	return err
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

// ListEvents — таймлайн расследования. Агенту он нужен, чтобы узнать id
// привязанных событий перед постановкой нод на граф. Фильтры entity_id и q,
// а также курсорная пагинация не реализованы — отдаётся первая страница.
func (s *Server) ListEvents(ctx context.Context, request events.ListEventsRequestObject) (events.ListEventsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Params.EntityId != nil || request.Params.Q != nil || request.Params.Cursor != nil {
		return nil, httperr.ErrNotImplemented
	}

	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"limit must be between 1 and 200")
	}

	items, err := s.db.InvestigationEvents(ctx, scope.ProjectID, request.InvestigationId.String(),
		model.EventFilter{
			EventType:  request.Params.EventType,
			SourceCode: request.Params.SourceCode,
			From:       request.Params.From,
			To:         request.Params.To,
			Limit:      limit,
		})
	if err != nil {
		return nil, storeError(err)
	}

	page := events.EventPage{Items: make([]events.EventSummary, 0, len(items))}
	for _, item := range items {
		converted, err := convertEventSummary(item)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, converted)
	}
	return events.ListEvents200JSONResponse(page), nil
}

func convertEventSummary(item model.EventSummary) (events.EventSummary, error) {
	id, err := dbUUID(item.ID)
	if err != nil {
		return events.EventSummary{}, err
	}

	var normalized map[string]any
	if len(item.NormalizedData) > 0 {
		if err := json.Unmarshal(item.NormalizedData, &normalized); err != nil {
			return events.EventSummary{}, fmt.Errorf("decode normalized_data: %w", err)
		}
	}

	attachedAt := item.AttachedAt
	attachedBy := events.Actor(item.AttachedBy)
	return events.EventSummary{
		Id:             id,
		SourceCode:     item.SourceCode,
		SourceEventId:  item.SourceEventID,
		SourceRef:      item.SourceRef,
		EventType:      item.EventType,
		OccurredAt:     item.OccurredAt,
		IngestedAt:     item.IngestedAt,
		AttachedAt:     &attachedAt,
		AttachedBy:     &attachedBy,
		Reason:         item.Reason,
		NormalizedData: &normalized,
	}, nil
}
