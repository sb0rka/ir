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
	request.Sources, err = server.constrainSources(r.Context(), request.Sources, domain.CapabilityEvents)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchEvents(r.Context(), projectAccess(r), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchEventsResponse{
		Events: eventsToAPI(result.Events), Entities: entitiesToAPI(result.Entities), Relations: relationsToAPI(result.Relations),
		SourceStates: sourceStatesToAPI(result.SourceStates), SourceErrors: sourceErrorsToAPI(result.SourceErrors),
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
	request, err := server.resolveContextRequest(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	for _, ref := range request.Findings {
		if err := server.requireSourceCapability(r, ref.SourceCode, domain.CapabilityFindings); err != nil {
			server.writeServiceError(w, err)
			return
		}
	}
	for _, ref := range request.Sessions {
		if err := server.requireSourceCapability(r, ref.SourceCode, domain.CapabilitySessions); err != nil {
			server.writeServiceError(w, err)
			return
		}
	}
	for _, ref := range request.Events {
		if err := server.requireSourceCapability(r, ref.SourceCode, domain.CapabilityEvents); err != nil {
			server.writeServiceError(w, err)
			return
		}
	}
	for _, ref := range request.Entities {
		if err := server.requireSourceCapability(r, ref.SourceCode, domain.CapabilityEvents); err != nil {
			server.writeServiceError(w, err)
			return
		}
	}
	result, err := server.service.ResolveContext(r.Context(), projectAccess(r), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, api.ResolveContextResponse{
		Findings: findingsToAPI(result.Findings), Sessions: sessionsToAPI(result.Sessions),
		Events: eventsToAPI(result.Events), Entities: entitiesToAPI(result.Entities), Relations: relationsToAPI(result.Relations),
		Resolutions: resolutionsToAPI(result.Resolutions), SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	})
}

func (server *Server) resolveContextRequest(body api.ResolveContextRequest) (service.ResolveContextRequest, error) {
	findings := valueOrEmpty(body.Findings)
	sessions := valueOrEmpty(body.Sessions)
	events := valueOrEmpty(body.Events)
	entities := valueOrEmpty(body.Entities)
	if len(findings)+len(sessions)+len(events)+len(entities) == 0 {
		return service.ResolveContextRequest{}, fmt.Errorf("at least one finding, session, event, or entity is required")
	}
	if len(findings) > 100 || len(sessions) > 100 || len(events) > 500 || len(entities) > 2000 {
		return service.ResolveContextRequest{}, fmt.Errorf("context selection exceeds its item limit")
	}
	request := service.ResolveContextRequest{
		Findings: make([]domain.SourceObjectRef, 0, len(findings)), Sessions: make([]domain.SourceObjectRef, 0, len(sessions)),
		Events: make([]domain.EventSourceRef, 0, len(events)), Entities: make([]domain.EntitySourceRef, 0, len(entities)),
	}
	seen := make(map[string]struct{}, len(findings)+len(sessions)+len(events)+len(entities))
	for _, value := range findings {
		ref, err := server.sourceObjectRefFromAPI(value)
		if err != nil {
			return service.ResolveContextRequest{}, err
		}
		if ref.RecordType == "nad_session" {
			return service.ResolveContextRequest{}, fmt.Errorf("nad_session belongs in sessions")
		}
		if err := validateFindingRef(server, ref.SourceCode, ref.RecordType, ref.SourceInstance, ref.ExternalID); err != nil {
			return service.ResolveContextRequest{}, err
		}
		key := "finding\x00" + ref.SourceCode + "\x00" + ref.SourceInstance + "\x00" + ref.RecordType + "\x00" + ref.ExternalID
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, fmt.Errorf("object references must be unique by source identity")
		}
		seen[key] = struct{}{}
		request.Findings = append(request.Findings, ref)
	}
	for _, value := range sessions {
		ref, err := server.sourceObjectRefFromAPI(value)
		if err != nil {
			return service.ResolveContextRequest{}, err
		}
		if ref.RecordType != "nad_session" || ref.SourceCode != "pt-nad" || !server.sourceInstanceAllowed(ref.SourceCode, ref.SourceInstance) {
			return service.ResolveContextRequest{}, fmt.Errorf("session reference must name a configured PT NAD store")
		}
		key := "session\x00" + ref.SourceCode + "\x00" + ref.SourceInstance + "\x00" + ref.ExternalID
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, fmt.Errorf("object references must be unique by source identity")
		}
		seen[key] = struct{}{}
		request.Sessions = append(request.Sessions, ref)
	}
	for _, ref := range events {
		source, id := strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceEventId)
		if source == "" || id == "" {
			return service.ResolveContextRequest{}, fmt.Errorf("event source_code and source_event_id are required")
		}
		key := "event\x00" + source + "\x00" + id
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, fmt.Errorf("event references must be unique")
		}
		seen[key] = struct{}{}
		request.Events = append(request.Events, domain.EventSourceRef{SourceCode: source, SourceEventID: id})
	}
	for _, ref := range entities {
		source, id := strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceEntityId)
		if source == "" || id == "" {
			return service.ResolveContextRequest{}, fmt.Errorf("entity source_code and source_entity_id are required")
		}
		key := "entity\x00" + source + "\x00" + id
		if _, duplicate := seen[key]; duplicate {
			return service.ResolveContextRequest{}, fmt.Errorf("entity references must be unique")
		}
		seen[key] = struct{}{}
		request.Entities = append(request.Entities, domain.EntitySourceRef{SourceCode: source, SourceEntityID: id})
	}
	return request, nil
}

func (server *Server) sourceObjectRefFromAPI(value api.SourceObjectRef) (domain.SourceObjectRef, error) {
	timeRange, err := objectTimeRange(value.TimeRange.From, value.TimeRange.To)
	if err != nil {
		return domain.SourceObjectRef{}, err
	}
	ref := domain.SourceObjectRef{
		SourceCode: strings.TrimSpace(value.SourceCode), SourceInstance: strings.TrimSpace(stringValue(value.SourceInstance)),
		RecordType: string(value.RecordType), ExternalID: strings.TrimSpace(value.ExternalId), TimeRange: timeRange,
	}
	if ref.SourceCode == "" || ref.ExternalID == "" {
		return domain.SourceObjectRef{}, fmt.Errorf("source_code and external_id are required")
	}
	return ref, nil
}

func (server *Server) requireSourceCapability(r *http.Request, source string, capabilityName domain.Capability) error {
	if !server.sourceAllowed(r.Context(), source) {
		return sourceForbidden(source)
	}
	if !server.service.Supports(source, capabilityName) {
		return fmt.Errorf("%w: source %q does not support %s", domain.ErrUnsupportedCapability, source, capabilityName)
	}
	return nil
}

func searchEventsRequest(body api.SearchEventsRequest) (service.SearchEventsRequest, error) {
	if err := validateLimit(body.Limit); err != nil {
		return service.SearchEventsRequest{}, err
	}
	if body.Entities != nil && len(*body.Entities) > 100 {
		return service.SearchEventsRequest{}, fmt.Errorf("entities must not contain more than 100 items")
	}
	timeRange, err := objectTimeRange(body.TimeRange.From, body.TimeRange.To)
	if err != nil {
		return service.SearchEventsRequest{}, err
	}
	request := service.SearchEventsRequest{
		Sources: valueOrEmpty(body.Sources), TimeFrom: timeRange.From, TimeTo: timeRange.To,
		Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
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
