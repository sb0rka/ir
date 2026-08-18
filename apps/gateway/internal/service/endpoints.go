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
	Query   string
	Limit   int
	Cursor  string
}

type SearchEndpointsResult struct {
	Items        []domain.Endpoint
	NextCursor   string
	SourceErrors []domain.SourceError
}

func (service *Service) SearchEndpoints(ctx context.Context, request SearchEndpointsRequest) (SearchEndpointsResult, error) {
	providers, err := service.registry.Select(request.Sources, domain.CapabilityEndpoints)
	if err != nil {
		return SearchEndpointsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := endpointFingerprint(request.Sources, request.Query)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchEndpointsResult{}, err
	}
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
			sourceCtx, sourceCancel := context.WithTimeout(requestCtx, service.sourceTimeout)
			defer sourceCancel()
			page, callErr := provider.Endpoints.SearchEndpoints(sourceCtx, capability.SearchEndpointsRequest{
				Query: request.Query, Limit: limit, Cursor: state.Positions[provider.Source.Code],
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchEndpointsResult{}
	continuations := make(map[string]string)
	providerHasMore := false
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				continue
			}
			providerHasMore = providerHasMore || item.page.HasMore
			result.Items = append(result.Items, item.page.Items...)
			for index, endpoint := range item.page.Items {
				if index < len(item.page.Continuations) {
					continuations[endpointKey(endpoint)] = item.page.Continuations[index]
				}
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
	moreMerged := len(result.Items) > limit
	if moreMerged {
		result.Items = result.Items[:limit]
	}
	for _, endpoint := range result.Items {
		if continuation := continuations[endpointKey(endpoint)]; continuation != "" {
			state.Positions[endpoint.Provenance.Source] = continuation
		}
	}
	if moreMerged || providerHasMore {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchEndpointsResult{}, err
		}
	}
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func (service *Service) ListResponseActions(ctx context.Context, source, externalID string) ([]domain.ResponseAction, error) {
	provider, ok := service.registry.Provider(source)
	if !ok {
		return nil, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", source)}
	}
	if provider.ResponseCatalog == nil {
		return nil, fmt.Errorf("%w: source %q does not support response catalog", domain.ErrUnsupportedCapability, source)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
	defer cancel()
	return provider.ResponseCatalog.ListResponseActions(requestCtx, externalID)
}

func endpointKey(endpoint domain.Endpoint) string {
	return endpoint.Provenance.Source + "\x00" + endpoint.ExternalID
}

func endpointFingerprint(sources []string, query string) string {
	sources = append([]string(nil), sources...)
	sort.Strings(sources)
	sum := sha256.Sum256([]byte(strings.Join(sources, ",") + "\x00" + strings.TrimSpace(query)))
	return hex.EncodeToString(sum[:])
}
