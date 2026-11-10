package ptnad

import (
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (provider *Provider) EventQueryDescription() capability.EventQueryDescription {
	return capability.EventQueryDescription{Languages: []string{}, Fields: []capability.EventField{}, CommonFilter: true, Bounded: true}
}
func (provider *Provider) ValidateEventQuery(request capability.SearchEventsRequest, aggregate bool) error {
	if request.FilterLanguage != "" || hasSourceEventControls(request) || aggregate {
		return &domain.RequestError{Code: "unsupported_query_control", Message: "PT NAD events support common filters only; native filters and aggregation are unavailable"}
	}
	return nil
}
