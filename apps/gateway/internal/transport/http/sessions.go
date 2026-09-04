package httptransport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

func (server *Server) SearchSessions(w http.ResponseWriter, r *http.Request, _ api.SearchSessionsParams) {
	var body api.SearchSessionsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateLimit(body.Limit); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	timeRange, err := objectTimeRange(body.TimeRange.From, body.TimeRange.To)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sources, err := server.constrainSources(r.Context(), valueOrEmpty(body.Sources), domain.CapabilitySessions)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchSessions(r.Context(), projectAccess(r), service.SearchSessionsRequest{
		Sources: sources, TimeRange: timeRange, Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchSessionsResponse{
		Sessions: sessionsToAPI(result.Sessions), SourceStates: sourceStatesToAPI(result.SourceStates), SourceErrors: sourceErrorsToAPI(result.SourceErrors),
		Total: result.Total,
	}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) GetSession(w http.ResponseWriter, r *http.Request, source, externalID string, params api.GetSessionParams) {
	if !server.sourceAllowed(r.Context(), source) {
		server.writeServiceError(w, sourceForbidden(source))
		return
	}
	if source != config.PTNAD || !server.service.Supports(source, domain.CapabilitySessions) {
		server.writeServiceError(w, fmt.Errorf("%w: source %q does not support sessions", domain.ErrUnsupportedCapability, source))
		return
	}
	if strings.TrimSpace(externalID) == "" || len(externalID) > 512 || !server.sourceInstanceAllowed(source, params.SourceInstance) {
		respondError(w, http.StatusBadRequest, "bad_request", "NAD session reference is not configured")
		return
	}
	timeRange, err := objectTimeRange(params.From, params.To)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	session, resolution, err := server.service.GetSession(r.Context(), projectAccess(r), domain.SourceObjectRef{
		SourceCode: source, SourceInstance: params.SourceInstance, RecordType: "nad_session", ExternalID: externalID, TimeRange: timeRange,
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": sessionsToAPI([]domain.Session{session})[0], "resolution": resolutionsToAPI([]domain.ObjectResolution{resolution})[0]})
}
