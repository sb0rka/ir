package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type SourceEventFields struct {
	SourceCode     string                  `json:"source_code"`
	SourceName     string                  `json:"source_name"`
	Languages      []string                `json:"languages"`
	Fields         []capability.EventField `json:"fields"`
	CommonFilter   bool                    `json:"common_filter"`
	Bounded        bool                    `json:"bounded"`
	CommonMappings *fieldmap.Source        `json:"common_mappings,omitempty"`
}
type EventFieldsResult struct {
	MappingVersion string              `json:"mapping_version"`
	CommonFields   []fieldmap.Field    `json:"common_fields"`
	Sources        []SourceEventFields `json:"sources"`
}

func (service *Service) EventFields(_ context.Context, allowed []string) (EventFieldsResult, error) {
	catalog, err := fieldmap.Default()
	if err != nil {
		return EventFieldsResult{}, err
	}
	result := EventFieldsResult{MappingVersion: fieldmap.Hash(), CommonFields: catalog.Fields, Sources: []SourceEventFields{}}
	for _, code := range allowed {
		p, ok := service.registry.Provider(code)
		if !ok || p.Events == nil {
			continue
		}
		planner, ok := p.Events.(capability.EventQueryPlanner)
		if !ok {
			continue
		}
		d := planner.EventQueryDescription()
		item := SourceEventFields{SourceCode: code, SourceName: p.Source.Name, Languages: d.Languages, Fields: d.Fields, CommonFilter: d.CommonFilter, Bounded: d.Bounded}
		if mapping, ok := catalog.Sources[code]; ok {
			copied := mapping
			if copied.Host == nil {
				copied.Host = []fieldmap.Path{}
			}
			if copied.IP == nil {
				copied.IP = []fieldmap.Path{}
			}
			item.CommonMappings = &copied
		}
		result.Sources = append(result.Sources, item)
	}
	return result, nil
}
func (service *Service) planEventQuery(sources []string, request capability.SearchEventsRequest, aggregate bool) ([]registry.Provider, error) {
	providers, err := service.registry.Select(sources, domain.CapabilityEvents)
	if err != nil {
		return nil, err
	}
	// Legacy clients retain provider-local validation and partial source errors.
	// Explicit languages/common conditions use the strict shared preflight below.
	if request.FilterLanguage == "" && request.CommonFilter.Empty() {
		return providers, nil
	}
	selected := []registry.Provider{}
	for _, p := range providers {
		planner, ok := p.Events.(capability.EventQueryPlanner)
		if !ok {
			if request.FilterLanguage != "" || !request.CommonFilter.Empty() {
				continue
			}
			selected = append(selected, p)
			continue
		}
		description := planner.EventQueryDescription()
		if request.FilterLanguage != "" && !slices.Contains(description.Languages, request.FilterLanguage) {
			continue
		}
		if err := planner.ValidateEventQuery(request, aggregate); err != nil {
			var bad *domain.RequestError
			if errors.As(err, &bad) {
				return nil, &domain.RequestError{Code: bad.Code, Message: p.Source.Code + ": " + bad.Message}
			}
			return nil, &domain.RequestError{Code: "unsupported_query_control", Message: p.Source.Code + ": unsupported event query"}
		}
		selected = append(selected, p)
	}
	if len(selected) > 0 {
		return selected, nil
	}
	return nil, &domain.RequestError{Code: "unsupported_filter_language", Message: fmt.Sprintf("no selected source supports filter language %q", request.FilterLanguage)}
}
