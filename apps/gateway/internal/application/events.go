package application

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
