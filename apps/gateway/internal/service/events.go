package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

type SearchEventsRequest struct {
	Sources  []string
	TimeFrom time.Time
	TimeTo   time.Time
	Entities []domain.EntityRef
	Limit    int
	Cursor   string
}

type SearchEventsResult struct {
	Events       []domain.Event
	Entities     []domain.Entity
	Relations    []domain.Relation
	NextCursor   string
	SourceStates []domain.SourceState
	SourceErrors []domain.SourceError
}

func (service *Service) SearchEvents(ctx context.Context, access ProjectAccess, request SearchEventsRequest) (SearchEventsResult, error) {
	selectedProviders, err := service.registry.Select(request.Sources, domain.CapabilityEvents)
	if err != nil {
		return SearchEventsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := requestFingerprint(request.Sources, request.TimeFrom, request.TimeTo, request.Entities)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchEventsResult{}, err
	}
	positions := state.Positions
	state.Positions = make(map[string]string)
	providers := pendingProviders(selectedProviders, state.Terminal)
	if len(providers) == 0 {
		return SearchEventsResult{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is exhausted"}
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
			var page capability.EventPage
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				page, innerErr = provider.Events.SearchEvents(attemptCtx, providerAccess, capability.SearchEventsRequest{
					TimeFrom: request.TimeFrom, TimeTo: request.TimeTo, Entities: request.Entities,
					Limit: limit, Cursor: positions[provider.Source.Code],
				})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchEventsResult{}
	for _, provider := range selectedProviders {
		if status := state.Terminal[provider.Source.Code]; status != "" {
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: provider.Source.Code, Status: status})
		}
	}
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: "failed"})
				continue
			}
			status := item.page.Status
			if status == "" {
				status = "complete"
			}
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: status})
			result.Events = append(result.Events, item.page.Events...)
			result.Entities = append(result.Entities, item.page.Entities...)
			result.Relations = append(result.Relations, item.page.Relations...)
			if item.page.NextCursor != "" {
				state.Positions[item.source] = item.page.NextCursor
			} else {
				state.Terminal[item.source] = status
			}
		case <-requestCtx.Done():
			for source := range pending {
				result.SourceStates = append(result.SourceStates, domain.SourceState{Source: source, Status: "failed"})
			}
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(result.SourceErrors) == len(selectedProviders) {
		return SearchEventsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}

	result.Events = normalization.Events(result.Events)
	if len(result.Events) > limit {
		result.Events = result.Events[:limit]
		markCompleteStatesTruncated(result.SourceStates)
		state.Positions = make(map[string]string)
	}
	selectedEntities := make(map[string]struct{})
	for _, event := range result.Events {
		for _, entity := range event.Entities {
			selectedEntities[entityKey(entity.EntityRef)] = struct{}{}
		}
	}
	result.Entities = filterEntities(normalization.Entities(result.Entities), selectedEntities)
	result.Relations = filterRelations(normalization.Relations(result.Relations), selectedEntities)
	if len(state.Positions) > 0 {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchEventsResult{}, err
		}
	}
	sortSourceStates(result.SourceStates)
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func requestFingerprint(sources []string, from, to time.Time, entities []domain.EntityRef) string {
	sources = append([]string(nil), sources...)
	sort.Strings(sources)
	entityKeys := make([]string, 0, len(entities))
	for _, entity := range entities {
		entityKeys = append(entityKeys, strings.ToLower(entity.Type)+":"+domain.CanonicalValue(entity.Type, entity.Value))
	}
	sort.Strings(entityKeys)
	raw := strings.Join(sources, ",") + "\x00" + from.UTC().Format(time.RFC3339Nano) + "\x00" +
		to.UTC().Format(time.RFC3339Nano) + "\x00" + strings.Join(entityKeys, ",")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func filterEntities(items []domain.Entity, selected map[string]struct{}) []domain.Entity {
	result := make([]domain.Entity, 0, len(items))
	for _, item := range items {
		if _, ok := selected[entityKey(domain.EntityRef{Type: item.Type, Value: item.Value})]; ok {
			result = append(result, item)
		}
	}
	return result
}

func filterRelations(items []domain.Relation, selected map[string]struct{}) []domain.Relation {
	result := make([]domain.Relation, 0, len(items))
	for _, item := range items {
		_, sourceOK := selected[entityKey(item.SourceEntity)]
		_, targetOK := selected[entityKey(item.TargetEntity)]
		if sourceOK && targetOK {
			result = append(result, item)
		}
	}
	return result
}

func entityKey(entity domain.EntityRef) string {
	kind := strings.ToLower(strings.TrimSpace(entity.Type))
	return kind + "\x00" + domain.CanonicalValue(kind, entity.Value)
}

func sortSourceStates(items []domain.SourceState) {
	sort.Slice(items, func(i, j int) bool { return items[i].Source < items[j].Source })
}

func markCompleteStatesTruncated(items []domain.SourceState) {
	for index := range items {
		if items[index].Status == "complete" {
			items[index].Status = "truncated"
		}
	}
}
