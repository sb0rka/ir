package httptransport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/application"
)

func (server *Server) SearchEndpoints(w http.ResponseWriter, r *http.Request, _ api.SearchEndpointsParams) {
	var body api.SearchEndpointsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateLimit(body.Limit); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sources, err := server.constrainSources(r.Context(), valueOrEmpty(body.Sources))
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchEndpoints(r.Context(), application.SearchEndpointsRequest{
		Sources: sources, Query: stringValue(body.Query), Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchEndpointsResponse{Items: endpointsToAPI(result.Items), SourceErrors: sourceErrorsToAPI(result.SourceErrors)}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) ListResponseActions(w http.ResponseWriter, r *http.Request, source, externalID string, _ api.ListResponseActionsParams) {
	if !server.sourceAllowed(r.Context(), source) {
		server.writeServiceError(w, sourceForbidden(source))
		return
	}
	items, err := server.service.ListResponseActions(r.Context(), source, externalID)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := make([]api.ResponseAction, 0, len(items))
	for _, item := range items {
		response = append(response, api.ResponseAction{Code: item.Code, Title: item.Title, Destructive: item.Destructive, Enabled: item.Enabled})
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": response})
}
