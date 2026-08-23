package httptransport

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

func (server *Server) SearchEvents(w http.ResponseWriter, r *http.Request, _ api.SearchEventsParams) {
	var body api.SearchEventsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request, err := searchEventsRequest(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request.Sources, err = server.constrainSources(r.Context(), request.Sources, domain.CapabilityEvents)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchEvents(r.Context(), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchEventsResponse{
		Events:       eventsToAPI(result.Events),
		Entities:     entitiesToAPI(result.Entities),
		Relations:    relationsToAPI(result.Relations),
		SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) ResolveContext(w http.ResponseWriter, r *http.Request, _ api.ResolveContextParams) {
	var body api.ResolveContextRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request, sources, err := resolveContextRequest(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if _, err = server.constrainSources(r.Context(), sources, domain.CapabilityEvents); err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.ResolveContext(r.Context(), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, api.ResolveContextResponse{
		Events: eventsToAPI(result.Events), Entities: entitiesToAPI(result.Entities),
		Relations: relationsToAPI(result.Relations), SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	})
}

func resolveContextRequest(body api.ResolveContextRequest) (service.ResolveContextRequest, []string, error) {
	if len(body.Events) == 0 && len(body.Entities) == 0 {
		return service.ResolveContextRequest{}, nil, fmt.Errorf("at least one event or entity is required")
	}
	if len(body.Events) > 500 {
		return service.ResolveContextRequest{}, nil, fmt.Errorf("events must not contain more than 500 items")
	}
	if len(body.Entities) > 2000 {
		return service.ResolveContextRequest{}, nil, fmt.Errorf("entities must not contain more than 2000 items")
	}
	request := service.ResolveContextRequest{
		Events:   make([]domain.EventSourceRef, 0, len(body.Events)),
		Entities: make([]domain.EntitySourceRef, 0, len(body.Entities)),
	}
	sourceSet := make(map[string]struct{})
	seen := make(map[string]struct{}, len(body.Events)+len(body.Entities))
	for _, ref := range body.Events {
		source, id := strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceEventId)
		if source == "" || id == "" {
			return service.ResolveContextRequest{}, nil, fmt.Errorf("event source_code and source_event_id are required")
		}
		key := "event\x00" + source + "\x00" + id
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, nil, fmt.Errorf("event references must be unique")
		}
		seen[key], sourceSet[source] = struct{}{}, struct{}{}
		request.Events = append(request.Events, domain.EventSourceRef{SourceCode: source, SourceEventID: id})
	}
	for _, ref := range body.Entities {
		source, id := strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceEntityId)
		if source == "" || id == "" {
			return service.ResolveContextRequest{}, nil, fmt.Errorf("entity source_code and source_entity_id are required")
		}
		key := "entity\x00" + source + "\x00" + id
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, nil, fmt.Errorf("entity references must be unique")
		}
		seen[key], sourceSet[source] = struct{}{}, struct{}{}
		request.Entities = append(request.Entities, domain.EntitySourceRef{SourceCode: source, SourceEntityID: id})
	}
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	return request, sources, nil
}

func searchEventsRequest(body api.SearchEventsRequest) (service.SearchEventsRequest, error) {
	if err := validateLimit(body.Limit); err != nil {
		return service.SearchEventsRequest{}, err
	}
	if body.Query != nil && utf8.RuneCountInString(*body.Query) > 1000 {
		return service.SearchEventsRequest{}, fmt.Errorf("query must not exceed 1000 characters")
	}
	if body.Entities != nil && len(*body.Entities) > 100 {
		return service.SearchEventsRequest{}, fmt.Errorf("entities must not contain more than 100 items")
	}
	request := service.SearchEventsRequest{
		Sources: valueOrEmpty(body.Sources), Query: stringValue(body.Query), Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	}
	if body.TimeRange != nil {
		if body.TimeRange.To.Before(body.TimeRange.From) {
			return service.SearchEventsRequest{}, fmt.Errorf("time_range.to must not precede time_range.from")
		}
		request.TimeFrom, request.TimeTo = body.TimeRange.From, body.TimeRange.To
	}
	if body.Entities != nil {
		request.Entities = make([]domain.EntityRef, 0, len(*body.Entities))
		for _, entity := range *body.Entities {
			if strings.TrimSpace(entity.Type) == "" || strings.TrimSpace(entity.Value) == "" {
				return service.SearchEventsRequest{}, fmt.Errorf("entity type and value are required")
			}
			request.Entities = append(request.Entities, domain.EntityRef{Type: entity.Type, Value: entity.Value})
		}
	}
	return request, nil
}
