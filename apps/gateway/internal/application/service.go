package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Service struct {
	registry       *registry.Registry
	requestTimeout time.Duration
	sourceTimeout  time.Duration
}

func New(registry *registry.Registry, requestTimeout, sourceTimeout time.Duration) *Service {
	return &Service{registry: registry, requestTimeout: requestTimeout, sourceTimeout: sourceTimeout}
}

type SearchEventsRequest struct {
	Sources  []string
	TimeFrom time.Time
	TimeTo   time.Time
	Query    string
	Entities []domain.EntityRef
	Limit    int
	Cursor   string
}

type SearchEventsResult struct {
	Events       []domain.Event
	Entities     []domain.Entity
	Relations    []domain.Relation
	NextCursor   string
	SourceErrors []domain.SourceError
}

type LookupEntityRequest struct {
	Sources []string
	Entity  domain.EntityRef
}

type LookupEntityResult struct {
	Entities     []domain.Entity
	Relations    []domain.Relation
	Verdicts     []domain.Verdict
	SourceErrors []domain.SourceError
}

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

type AllSourcesError struct {
	Items []domain.SourceError
}

func (err *AllSourcesError) Error() string { return domain.ErrAllSourcesFailed.Error() }
func (err *AllSourcesError) Unwrap() error { return domain.ErrAllSourcesFailed }

func (service *Service) ListSources() []domain.Source {
	return service.registry.Sources()
}

func (service *Service) SearchEvents(ctx context.Context, request SearchEventsRequest) (SearchEventsResult, error) {
	providers, err := service.registry.Select(request.Sources, domain.CapabilityEvents)
	if err != nil {
		return SearchEventsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := requestFingerprint(request.Sources, request.TimeFrom, request.TimeTo, request.Query, request.Entities)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchEventsResult{}, err
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
			page, callErr := provider.Events.SearchEvents(sourceCtx, capability.SearchEventsRequest{
				TimeFrom: request.TimeFrom,
				TimeTo:   request.TimeTo,
				Query:    request.Query,
				Entities: request.Entities,
				Limit:    limit,
				Cursor:   state.Positions[provider.Source.Code],
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchEventsResult{}
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
			result.Events = append(result.Events, item.page.Events...)
			result.Entities = append(result.Entities, item.page.Entities...)
			result.Relations = append(result.Relations, item.page.Relations...)
			for index, event := range item.page.Events {
				if index < len(item.page.Continuations) {
					continuations[eventKey(event)] = item.page.Continuations[index]
				}
			}
		case <-requestCtx.Done():
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(result.SourceErrors) == len(providers) {
		return SearchEventsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}

	result.Events = normalization.Events(result.Events)
	moreMerged := len(result.Events) > limit
	if moreMerged {
		result.Events = result.Events[:limit]
	}
	selectedEntityIDs := make(map[string]struct{})
	for _, event := range result.Events {
		for _, id := range event.EntityIDs {
			selectedEntityIDs[id.String()] = struct{}{}
		}
		if continuation := continuations[eventKey(event)]; continuation != "" {
			state.Positions[event.Provenance.Source] = continuation
		}
	}
	result.Entities = filterEntities(normalization.Entities(result.Entities), selectedEntityIDs)
	result.Relations = filterRelations(normalization.Relations(result.Relations), selectedEntityIDs)
	if moreMerged || providerHasMore {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchEventsResult{}, err
		}
	}
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func (service *Service) LookupEntity(ctx context.Context, request LookupEntityRequest) (LookupEntityResult, error) {
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
			sourceCtx, sourceCancel := context.WithTimeout(requestCtx, service.sourceTimeout)
			defer sourceCancel()
			value, callErr := provider.EntityLookup.LookupEntity(sourceCtx, capability.LookupEntityRequest{Entity: request.Entity})
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

func (service *Service) AnalyzeArtifact(ctx context.Context, source string, request capability.AnalyzeArtifactRequest) (domain.Analysis, error) {
	provider, ok := service.registry.Provider(source)
	if !ok {
		return domain.Analysis{}, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", source)}
	}
	if provider.ArtifactAnalyzer == nil {
		return domain.Analysis{}, fmt.Errorf("%w: source %q does not support artifact analysis", domain.ErrUnsupportedCapability, source)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
	defer cancel()
	return provider.ArtifactAnalyzer.AnalyzeArtifact(requestCtx, request)
}

func (service *Service) GetAnalysis(ctx context.Context, analysisID string) (domain.Analysis, error) {
	providers, err := service.registry.Select(nil, domain.CapabilityArtifactAnalysis)
	if err != nil {
		return domain.Analysis{}, err
	}
	for _, provider := range providers {
		requestCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
		analysis, callErr := provider.ArtifactAnalyzer.GetAnalysis(requestCtx, analysisID)
		cancel()
		if callErr == nil {
			return analysis, nil
		}
		if !errors.Is(callErr, domain.ErrNotFound) {
			return domain.Analysis{}, callErr
		}
	}
	return domain.Analysis{}, fmt.Errorf("%w: analysis %q", domain.ErrNotFound, analysisID)
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

type cursorState struct {
	Fingerprint string            `json:"fingerprint"`
	Positions   map[string]string `json:"positions"`
}

func decodeCursor(raw, fingerprint string) (cursorState, error) {
	state := cursorState{Fingerprint: fingerprint, Positions: map[string]string{}}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(decoded, &state) != nil || state.Positions == nil {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is invalid"}
	}
	if state.Fingerprint != fingerprint {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor does not match the request"}
	}
	return state, nil
}

func encodeCursor(state cursorState) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func requestFingerprint(sources []string, from, to time.Time, query string, entities []domain.EntityRef) string {
	sources = append([]string(nil), sources...)
	sort.Strings(sources)
	entityKeys := make([]string, 0, len(entities))
	for _, entity := range entities {
		entityKeys = append(entityKeys, strings.ToLower(entity.Type)+":"+domain.CanonicalValue(entity.Type, entity.Value))
	}
	sort.Strings(entityKeys)
	raw := strings.Join(sources, ",") + "\x00" + from.UTC().Format(time.RFC3339Nano) + "\x00" +
		to.UTC().Format(time.RFC3339Nano) + "\x00" + strings.TrimSpace(query) + "\x00" + strings.Join(entityKeys, ",")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func eventKey(event domain.Event) string {
	return event.Provenance.Source + "\x00" + event.Provenance.ExternalID
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

func filterEntities(items []domain.Entity, selected map[string]struct{}) []domain.Entity {
	result := make([]domain.Entity, 0, len(items))
	for _, item := range items {
		if _, ok := selected[item.ID.String()]; ok {
			result = append(result, item)
		}
	}
	return result
}

func filterRelations(items []domain.Relation, selected map[string]struct{}) []domain.Relation {
	result := make([]domain.Relation, 0, len(items))
	for _, item := range items {
		_, sourceOK := selected[item.SourceEntityID.String()]
		_, targetOK := selected[item.TargetEntityID.String()]
		if sourceOK && targetOK {
			result = append(result, item)
		}
	}
	return result
}

func normalizeLimit(value int) int {
	if value < 1 || value > 100 {
		return 50
	}
	return value
}

func sourceError(source string, err error) domain.SourceError {
	item := domain.SourceError{Source: source, Code: "provider_error", Message: "source request failed", Retryable: true}
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		item.Code = requestErr.Code
		item.Message = requestErr.Message
		item.Retryable = false
		return item
	}
	if errors.Is(err, context.DeadlineExceeded) {
		item.Code = "source_timeout"
		item.Message = "source request timed out"
	}
	return item
}

func sortSourceErrors(items []domain.SourceError) {
	sort.Slice(items, func(i, j int) bool { return items[i].Source < items[j].Source })
}

func providerCodes(providers []registry.Provider) map[string]struct{} {
	result := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		result[provider.Source.Code] = struct{}{}
	}
	return result
}

func appendPendingErrors(items *[]domain.SourceError, pending map[string]struct{}, err error) {
	for source := range pending {
		*items = append(*items, sourceError(source, err))
	}
}
