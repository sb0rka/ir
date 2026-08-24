package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

type SearchEndpointsRequest struct {
	Sources []string
	Limit   int
	Cursor  string
}

type SearchEndpointsResult struct {
	Items        []domain.Endpoint
	NextCursor   string
	SourceErrors []domain.SourceError
}

func (service *Service) SearchEndpoints(ctx context.Context, access ProjectAccess, request SearchEndpointsRequest) (SearchEndpointsResult, error) {
	providers, err := service.registry.Select(request.Sources, domain.CapabilityEndpoints)
	if err != nil {
		return SearchEndpointsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := endpointFingerprint(request.Sources)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchEndpointsResult{}, err
	}
	positions := state.Positions
	state.Positions = make(map[string]string)
	type providerResult struct {
		source string
		page   capability.EndpointPage
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			var page capability.EndpointPage
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				page, innerErr = provider.Endpoints.SearchEndpoints(attemptCtx, providerAccess, capability.SearchEndpointsRequest{
					Limit: limit, Cursor: positions[provider.Source.Code],
				})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchEndpointsResult{}
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				continue
			}
			result.Items = append(result.Items, item.page.Items...)
			if item.page.NextCursor != "" {
				state.Positions[item.source] = item.page.NextCursor
			}
		case <-requestCtx.Done():
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(result.SourceErrors) == len(providers) {
		return SearchEndpointsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	result.Items = normalization.Endpoints(result.Items)
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	if len(state.Positions) > 0 {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchEndpointsResult{}, err
		}
	}
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func (service *Service) ListResponseActions(ctx context.Context, access ProjectAccess, source, externalID string) ([]domain.ResponseAction, error) {
	provider, ok := service.registry.Provider(source)
	if !ok {
		return nil, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", source)}
	}
	if provider.ResponseCatalog == nil {
		return nil, fmt.Errorf("%w: source %q does not support response catalog", domain.ErrUnsupportedCapability, source)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	var items []domain.ResponseAction
	err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
		var innerErr error
		items, innerErr = provider.ResponseCatalog.ListResponseActions(attemptCtx, providerAccess, externalID)
		return innerErr
	})
	return items, err
}

func endpointFingerprint(sources []string) string {
	sources = append([]string(nil), sources...)
	sort.Strings(sources)
	sum := sha256.Sum256([]byte(strings.Join(sources, ",")))
	return hex.EncodeToString(sum[:])
}
