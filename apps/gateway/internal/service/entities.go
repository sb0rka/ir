package service

import (
	"context"
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

type LookupEntityRequest struct {
	Sources   []string
	Entity    domain.EntityRef
	TimeRange domain.TimeRange
}

type LookupEntityResult struct {
	Entities     []domain.Entity
	Relations    []domain.Relation
	Verdicts     []domain.Verdict
	SourceErrors []domain.SourceError
}

func (service *Service) LookupEntity(ctx context.Context, access ProjectAccess, request LookupEntityRequest) (LookupEntityResult, error) {
	providers, err := service.registry.Select(request.Sources, domain.CapabilityEntityLookup)
	if err != nil {
		return LookupEntityResult{}, err
	}
	type providerResult struct {
		source string
		value  capability.LookupEntityResult
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			var value capability.LookupEntityResult
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				value, innerErr = provider.EntityLookup.LookupEntity(attemptCtx, providerAccess, capability.LookupEntityRequest{Entity: request.Entity, TimeRange: request.TimeRange})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, value: value, err: callErr}
		}()
	}

	result := LookupEntityResult{}
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				continue
			}
			result.Entities = append(result.Entities, item.value.Entities...)
			result.Relations = append(result.Relations, item.value.Relations...)
			result.Verdicts = append(result.Verdicts, item.value.Verdicts...)
		case <-requestCtx.Done():
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(result.SourceErrors) == len(providers) {
		return LookupEntityResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	result.Entities = normalization.Entities(result.Entities)
	result.Relations = normalization.Relations(result.Relations)
	sort.Slice(result.Verdicts, func(i, j int) bool { return result.Verdicts[i].Provider < result.Verdicts[j].Provider })
	sortSourceErrors(result.SourceErrors)
	return result, nil
}
