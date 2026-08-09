package httptransport

import (
	"fmt"
	"net/http"
	"strings"

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
	request.Sources, err = server.constrainSources(r.Context(), request.Sources)
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

func searchEventsRequest(body api.SearchEventsRequest) (service.SearchEventsRequest, error) {
	if err := validateLimit(body.Limit); err != nil {
		return service.SearchEventsRequest{}, err
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
