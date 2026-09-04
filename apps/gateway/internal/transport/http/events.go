package httptransport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
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
		Total: result.Total,
	}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) AggregateEvents(w http.ResponseWriter, r *http.Request, _ api.AggregateEventsParams) {
	var body api.AggregateEventsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request, err := aggregateEventsRequest(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request.Sources, err = server.constrainSources(r.Context(), request.Sources, domain.CapabilityEvents)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.AggregateEvents(r.Context(), projectAccess(r), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, api.AggregateEventsResponse{
		Groups:       eventGroupsToAPI(result.Groups),
		SourceStates: sourceStatesToAPI(result.SourceStates),
		SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	})
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
		Filter: strings.TrimSpace(stringValue(body.Filter)), Columns: trimmedValues(body.Columns),
		GroupBy: trimmedValues(body.GroupBy), GroupValues: valueOrEmpty(body.GroupValues),
		Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	}
	if err := validateEventSearchControls(request, body.GroupValues != nil); err != nil {
		return service.SearchEventsRequest{}, err
	}
	if body.Sort != nil {
		request.Sort = make([]capability.EventSort, 0, len(*body.Sort))
		for _, item := range *body.Sort {
			request.Sort = append(request.Sort, capability.EventSort{Field: strings.TrimSpace(item.Field), Direction: string(item.Direction)})
		}
	}
	if err := validateEventSort(request.Sort); err != nil {
		return service.SearchEventsRequest{}, err
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

func aggregateEventsRequest(body api.AggregateEventsRequest) (service.AggregateEventsRequest, error) {
	if body.Limit != nil && (*body.Limit < 1 || *body.Limit > 1000) {
		return service.AggregateEventsRequest{}, fmt.Errorf("limit must be between 1 and 1000")
	}
	if len(body.GroupBy) < 1 || len(body.GroupBy) > 8 {
		return service.AggregateEventsRequest{}, fmt.Errorf("group_by must contain between 1 and 8 fields")
	}
	if body.Entities != nil && len(*body.Entities) > 100 {
		return service.AggregateEventsRequest{}, fmt.Errorf("entities must not contain more than 100 items")
	}
	timeRange, err := objectTimeRange(body.TimeRange.From, body.TimeRange.To)
	if err != nil {
		return service.AggregateEventsRequest{}, err
	}
	request := service.AggregateEventsRequest{
		Sources: valueOrEmpty(body.Sources), TimeRange: timeRange,
		Filter: strings.TrimSpace(stringValue(body.Filter)), GroupBy: trimmedValues(&body.GroupBy),
		Limit: intValue(body.Limit),
	}
	if len(request.Filter) > 4096 || strings.ContainsAny(request.Filter, "|;\r\n\x00") ||
		strings.Contains(request.Filter, "--") || strings.Contains(request.Filter, "/*") || strings.Contains(request.Filter, "*/") {
		return service.AggregateEventsRequest{}, fmt.Errorf("filter must be a predicate without pipeline separators, comments, or control characters")
	}
	if err := validateUniqueNonEmpty("group_by", request.GroupBy); err != nil {
		return service.AggregateEventsRequest{}, err
	}
	for _, field := range request.GroupBy {
		if len(field) > 128 {
			return service.AggregateEventsRequest{}, fmt.Errorf("group_by fields must not exceed 128 characters")
		}
	}
	if body.Sort != nil {
		if len(*body.Sort) > 8 {
			return service.AggregateEventsRequest{}, fmt.Errorf("sort must not contain more than 8 fields")
		}
		request.Sort = make([]capability.EventSort, 0, len(*body.Sort))
		for _, item := range *body.Sort {
			request.Sort = append(request.Sort, capability.EventSort{Field: strings.TrimSpace(item.Field), Direction: string(item.Direction)})
		}
	}
	if err := validateAggregationSort(request.Sort, request.GroupBy); err != nil {
		return service.AggregateEventsRequest{}, err
	}
	if body.Entities != nil {
		request.Entities = make([]domain.EntityRef, 0, len(*body.Entities))
		for _, entity := range *body.Entities {
			if strings.TrimSpace(entity.Type) == "" || strings.TrimSpace(entity.Value) == "" {
				return service.AggregateEventsRequest{}, fmt.Errorf("entity type and value are required")
			}
			request.Entities = append(request.Entities, domain.EntityRef{Type: entity.Type, Value: entity.Value})
		}
	}
	return request, nil
}

func validateAggregationSort(values []capability.EventSort, groupBy []string) error {
	groups := make(map[string]struct{}, len(groupBy))
	for _, field := range groupBy {
		groups[field] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.Field == "" {
			return fmt.Errorf("sort field is required")
		}
		if value.Field != "count" {
			if _, grouped := groups[value.Field]; !grouped {
				return fmt.Errorf("sort field must be count or a group_by field")
			}
		}
		if _, duplicate := seen[value.Field]; duplicate {
			return fmt.Errorf("sort fields must be unique")
		}
		seen[value.Field] = struct{}{}
		if value.Direction != "asc" && value.Direction != "desc" {
			return fmt.Errorf("sort direction must be asc or desc")
		}
	}
	return nil
}

func eventGroupsToAPI(values []domain.EventGroup) []api.EventGroup {
	result := make([]api.EventGroup, 0, len(values))
	for _, value := range values {
		result = append(result, api.EventGroup{
			SourceCode: value.SourceCode,
			Values:     append([]*string(nil), value.Values...),
			Count:      value.Count,
		})
	}
	return result
}

func trimmedValues(values *[]string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, 0, len(*values))
	for _, value := range *values {
		result = append(result, strings.TrimSpace(value))
	}
	return result
}

func validateEventSearchControls(request service.SearchEventsRequest, hasGroupValues bool) error {
	if strings.ContainsAny(request.Filter, "|;\r\n\x00") {
		return fmt.Errorf("filter must be a predicate without pipeline separators or control characters")
	}
	if err := validateUniqueNonEmpty("columns", request.Columns); err != nil {
		return err
	}
	if err := validateUniqueNonEmpty("group_by", request.GroupBy); err != nil {
		return err
	}
	if len(request.GroupBy) > 0 && !hasGroupValues {
		return fmt.Errorf("group_values is required with group_by")
	}
	if hasGroupValues && len(request.GroupValues) != len(request.GroupBy) {
		return fmt.Errorf("group_values must contain one value for every group_by field")
	}
	if hasGroupValues && len(request.GroupBy) == 0 {
		return fmt.Errorf("group_values requires group_by")
	}
	for _, value := range request.GroupValues {
		if value != nil && strings.ContainsAny(*value, "\r\n\x00") {
			return fmt.Errorf("group_values must not contain control characters")
		}
	}
	return nil
}

func validateEventSort(values []capability.EventSort) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.Field == "" {
			return fmt.Errorf("sort field is required")
		}
		if _, exists := seen[value.Field]; exists {
			return fmt.Errorf("sort fields must be unique")
		}
		seen[value.Field] = struct{}{}
		if value.Direction != "asc" && value.Direction != "desc" {
			return fmt.Errorf("sort direction must be asc or desc")
		}
	}
	return nil
}

func validateUniqueNonEmpty(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("%s must not contain empty fields", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s fields must be unique", name)
		}
		seen[value] = struct{}{}
	}
	return nil
}
