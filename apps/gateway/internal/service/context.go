package service

import (
	"context"
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

type ResolveContextRequest struct {
	Events   []domain.EventSourceRef
	Entities []domain.EntitySourceRef
}

type ResolveContextResult struct {
	Events       []domain.Event
	Entities     []domain.Entity
	Relations    []domain.Relation
	SourceErrors []domain.SourceError
}

// ResolveContext retrieves current normalized records by their source-owned IDs.
// Each provider receives only IDs that belong to it; no computed Gateway ID is
// introduced between the client and the source system.
func (service *Service) ResolveContext(ctx context.Context, request ResolveContextRequest) (ResolveContextResult, error) {
	bySource := make(map[string]capability.ResolveContextRequest)
	for _, ref := range request.Events {
		item := bySource[ref.SourceCode]
		item.EventIDs = append(item.EventIDs, ref.SourceEventID)
		bySource[ref.SourceCode] = item
	}
	for _, ref := range request.Entities {
		item := bySource[ref.SourceCode]
		item.EntityIDs = append(item.EntityIDs, ref.SourceEntityID)
		bySource[ref.SourceCode] = item
	}
	sources := make([]string, 0, len(bySource))
	for source := range bySource {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	providers, err := service.registry.Select(sources, domain.CapabilityEvents)
	if err != nil {
		return ResolveContextResult{}, err
	}

	type providerResult struct {
		source string
		page   capability.EventPage
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			sourceCtx, sourceCancel := context.WithTimeout(requestCtx, service.sourceTimeout)
			defer sourceCancel()
			page, callErr := provider.Events.ResolveContext(sourceCtx, bySource[provider.Source.Code])
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := ResolveContextResult{}
	var onlyProviderError error
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				onlyProviderError = item.err
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				continue
			}
			result.Events = append(result.Events, item.page.Events...)
			result.Entities = append(result.Entities, item.page.Entities...)
			result.Relations = append(result.Relations, item.page.Relations...)
		case <-requestCtx.Done():
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(providers) == 1 && onlyProviderError != nil {
		return ResolveContextResult{}, onlyProviderError
	}
	if len(result.SourceErrors) == len(providers) {
		return ResolveContextResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	result.Events = normalization.Events(result.Events)
	result.Entities = normalization.Entities(result.Entities)
	result.Relations = normalization.Relations(result.Relations)
	sortSourceErrors(result.SourceErrors)
	return result, nil
}
