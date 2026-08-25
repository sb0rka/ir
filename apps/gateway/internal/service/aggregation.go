package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const (
	defaultAggregationLimit = 100
	maxAggregationLimit     = 1000
)

type AggregateEventsRequest struct {
	Sources   []string
	TimeRange domain.TimeRange
	Entities  []domain.EntityRef
	Filter    string
	GroupBy   []string
	Sort      []capability.EventSort
	Limit     int
}

type AggregateEventsResult struct {
	Groups       []domain.EventGroup
	SourceStates []domain.SourceState
	SourceErrors []domain.SourceError
}

func (service *Service) AggregateEvents(ctx context.Context, access ProjectAccess, request AggregateEventsRequest) (AggregateEventsResult, error) {
	selectedProviders, err := service.registry.Select(request.Sources, domain.CapabilityEvents)
	if err != nil {
		return AggregateEventsResult{}, err
	}
	limit, err := aggregationLimit(request.Limit)
	if err != nil {
		return AggregateEventsResult{}, err
	}
	sort.Slice(selectedProviders, func(left, right int) bool {
		return selectedProviders[left].Source.Code < selectedProviders[right].Source.Code
	})

	providers := make([]registry.Provider, 0, len(selectedProviders))
	result := AggregateEventsResult{}
	for _, provider := range selectedProviders {
		if provider.EventAggregation == nil {
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: provider.Source.Code, Status: "failed"})
			result.SourceErrors = append(result.SourceErrors, unsupportedAggregationError(provider.Source.Code))
			continue
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return AggregateEventsResult{}, fmt.Errorf("%w: no selected source supports event aggregation", domain.ErrUnsupportedCapability)
	}

	type providerResult struct {
		source string
		page   capability.EventGroupPage
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			var page capability.EventGroupPage
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				page, innerErr = provider.EventAggregation.AggregateEvents(attemptCtx, providerAccess, capability.AggregateEventsRequest{
					TimeFrom: request.TimeRange.From,
					TimeTo:   request.TimeRange.To,
					Entities: request.Entities,
					Filter:   request.Filter,
					GroupBy:  request.GroupBy,
					Sort:     request.Sort,
					Limit:    limit,
				})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	providerResults := make(map[string]providerResult, len(providers))
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			providerResults[item.source] = item
		case <-requestCtx.Done():
			for source := range pending {
				providerResults[source] = providerResult{source: source, err: requestCtx.Err()}
			}
			pending = nil
		}
	}

	succeeded := 0
	for _, provider := range selectedProviders {
		if provider.EventAggregation == nil {
			continue
		}
		item := providerResults[provider.Source.Code]
		if item.err != nil {
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: "failed"})
			result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
			continue
		}
		succeeded++
		status := item.page.Status
		if status == "" {
			status = "complete"
		}
		groups := item.page.Groups
		if len(groups) > limit {
			groups = groups[:limit]
			status = "truncated"
		}
		for _, group := range groups {
			group.SourceCode = item.source
			result.Groups = append(result.Groups, group)
		}
		result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: status})
	}
	if succeeded == 0 {
		sortSourceErrors(result.SourceErrors)
		return AggregateEventsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	sortSourceStates(result.SourceStates)
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func aggregationLimit(value int) (int, error) {
	if value == 0 {
		return defaultAggregationLimit, nil
	}
	if value < 1 || value > maxAggregationLimit {
		return 0, &domain.RequestError{Code: "invalid_limit", Message: "limit must be between 1 and 1000"}
	}
	return value, nil
}

func unsupportedAggregationError(source string) domain.SourceError {
	return domain.SourceError{
		Source:    source,
		Code:      "unsupported_event_aggregation",
		Message:   "source does not support event aggregation",
		Retryable: false,
	}
}
