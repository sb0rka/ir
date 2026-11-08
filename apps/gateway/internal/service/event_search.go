package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type eventStream struct {
	delivered int
	provider  registry.Provider
	position  *eventPosition
	page      capability.EventPage
	loaded    bool
	access    capability.Access
	err       error
	cleanup   string
}

func (stream *eventStream) fetch(ctx context.Context, service *Service, access ProjectAccess, request SearchEventsRequest) {
	if stream.loaded || stream.position.Done || stream.err != nil {
		return
	}
	stream.err = service.callProvider(ctx, access, stream.provider, func(ctx context.Context, credentials capability.Access) error {
		stream.access = credentials
		page, err := stream.provider.Events.SearchEvents(ctx, credentials, capability.SearchEventsRequest{TimeFrom: request.TimeFrom, TimeTo: request.TimeTo, Entities: request.Entities, Filter: request.Filter, Columns: request.Columns, Sort: request.Sort, GroupBy: request.GroupBy, GroupValues: request.GroupValues, CommonFilter: request.CommonFilter, FilterLanguage: request.FilterLanguage, Limit: stream.position.PageSize, Cursor: stream.position.PageCursor})
		if err == nil {
			stream.page = page
		}
		return err
	})
	if stream.err != nil {
		return
	}
	if len(stream.page.Events) > stream.position.PageSize || stream.position.Consumed > len(stream.page.Events) || len(stream.page.Events) == 0 && stream.page.NextCursor != "" || stream.page.NextCursor != "" && stream.page.NextCursor == stream.position.PageCursor {
		stream.err = &domain.UpstreamError{StatusCode: 502, Message: "source event page is invalid"}
		return
	}
	seen := map[string]bool{}
	for _, event := range stream.page.Events {
		if event.Provenance.Source != stream.provider.Source.Code || event.Provenance.ExternalID == "" || seen[event.Provenance.ExternalID] {
			stream.err = &domain.UpstreamError{StatusCode: 502, Message: "source event identity is invalid"}
			return
		}
		seen[event.Provenance.ExternalID] = true
	}
	if stream.page.ReplayCursor != "" {
		stream.position.PageCursor = stream.page.ReplayCursor
	}
	if stream.page.Total != nil {
		total := *stream.page.Total
		if total < 0 {
			stream.err = &domain.UpstreamError{StatusCode: 502, Message: "source total is invalid"}
			return
		}
		stream.position.Total = &total
	}
	stream.position.Status = stream.page.Status
	if stream.position.Status == "" {
		stream.position.Status = "complete"
	}
	stream.loaded = true
	if stream.position.Consumed == len(stream.page.Events) {
		stream.advance()
	}
}

func (stream *eventStream) advance() {
	if stream.page.NextCursor == "" {
		stream.position.Done = true
		stream.cleanup = stream.position.PageCursor
		stream.position.PageCursor = ""
	} else {
		stream.position.PageCursor = stream.page.NextCursor
	}
	stream.position.Consumed = 0
	stream.loaded = false
}

func (service *Service) SearchEvents(ctx context.Context, access ProjectAccess, request SearchEventsRequest) (SearchEventsResult, error) {
	normalized, err := fieldmap.Normalize(request.CommonFilter)
	if err != nil {
		return SearchEventsResult{}, err
	}
	request.CommonFilter = normalized
	providers, err := service.planEventQuery(request.Sources, capability.SearchEventsRequest{TimeFrom: request.TimeFrom, TimeTo: request.TimeTo, Entities: request.Entities, CommonFilter: request.CommonFilter, Filter: request.Filter, FilterLanguage: request.FilterLanguage, Columns: request.Columns, Sort: request.Sort, GroupBy: request.GroupBy, GroupValues: request.GroupValues, Limit: normalizeLimit(request.Limit)}, false)
	if err != nil {
		return SearchEventsResult{}, err
	}
	request.Sources = make([]string, 0, len(providers))
	for _, p := range providers {
		request.Sources = append(request.Sources, p.Source.Code)
	}
	state, err := service.decodeEventCursor(request.Cursor, access.ProjectID, requestFingerprint(request))
	if err != nil {
		return SearchEventsResult{}, err
	}
	if request.Cursor != "" && len(state.Sources) != len(providers) {
		return SearchEventsResult{}, invalidEventCursor()
	}
	limit := normalizeLimit(request.Limit)
	streams := make([]*eventStream, 0, len(providers))
	pending := false
	for _, provider := range providers {
		position, exists := state.Sources[provider.Source.Code]
		if !exists {
			if request.Cursor != "" {
				return SearchEventsResult{}, invalidEventCursor()
			}
			position = &eventPosition{PageSize: limit}
			state.Sources[provider.Source.Code] = position
		}
		if !position.Done {
			pending = true
		}
		streams = append(streams, &eventStream{provider: provider, position: position})
	}
	if !pending {
		return SearchEventsResult{}, invalidEventCursor()
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, stream := range streams {
		wg.Add(1)
		go func(s *eventStream) { defer wg.Done(); s.fetch(requestCtx, service, access, request) }(stream)
	}
	wg.Wait()
	result := SearchEventsResult{}
	appendContext := func(s *eventStream) {
		result.Entities = append(result.Entities, s.page.Entities...)
		result.Relations = append(result.Relations, s.page.Relations...)
	}
	for _, stream := range streams {
		if stream.err == nil && stream.loaded {
			appendContext(stream)
		}
	}
	for len(result.Events) < limit {
		var best *eventStream
		for _, stream := range streams {
			if !stream.loaded && !stream.position.Done && stream.err == nil {
				stream.fetch(requestCtx, service, access, request)
				if stream.err == nil && stream.loaded {
					appendContext(stream)
				}
			}
			if stream.err != nil {
				var requestErr *domain.RequestError
				if errors.As(stream.err, &requestErr) && requestErr.Code == "invalid_cursor" {
					return SearchEventsResult{}, invalidEventCursor()
				}
				continue
			}
			if !stream.loaded {
				continue
			}
			if best == nil || compareEvents(stream.page.Events[stream.position.Consumed], best.page.Events[best.position.Consumed], request.Sort) < 0 {
				best = stream
			}
		}
		if best == nil {
			break
		}
		result.Events = append(result.Events, best.page.Events[best.position.Consumed])
		best.position.Consumed++
		best.delivered++
		if best.position.Consumed == len(best.page.Events) {
			best.advance()
		}
	}
	hasNext := false
	for _, stream := range streams {
		status := stream.position.Status
		if stream.err != nil {
			status = "failed"
			if stream.delivered > 0 {
				status = "truncated"
			}
			result.SourceErrors = append(result.SourceErrors, sourceError(stream.provider.Source.Code, stream.err))
		}
		if status == "" {
			status = "complete"
		}
		result.SourceStates = append(result.SourceStates, domain.SourceState{Source: stream.provider.Source.Code, Status: status, Total: stream.position.Total})
		hasNext = hasNext || !stream.position.Done
	}
	if len(result.SourceErrors) == len(providers) && len(result.Events) == 0 {
		return SearchEventsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	if hasNext {
		result.NextCursor, err = service.encodeEventCursor(state)
		if err != nil {
			return SearchEventsResult{}, err
		}
	}
	selected := map[string]struct{}{}
	for _, event := range result.Events {
		for _, mention := range event.Entities {
			selected[entityKey(mention.EntityRef)] = struct{}{}
		}
	}
	result.Entities = filterEntities(normalization.Entities(result.Entities), selected)
	result.Relations = filterRelations(normalization.Relations(result.Relations), selected)
	sortSourceStates(result.SourceStates)
	sortSourceErrors(result.SourceErrors)
	result.Total = sumSourceTotals(result.SourceStates)
	for _, stream := range streams {
		if stream.cleanup == "" {
			continue
		}
		if closer, ok := stream.provider.Events.(capability.EventCursorCloser); ok {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			_ = closer.CloseEventCursor(cleanupCtx, stream.access, stream.cleanup)
			cancel()
		}
	}
	return result, nil
}

func requestFingerprint(request SearchEventsRequest) string {
	sources := append([]string(nil), request.Sources...)
	sort.Strings(sources)
	entityKeys := make([]string, 0, len(request.Entities))
	for _, entity := range request.Entities {
		entityKeys = append(entityKeys, strings.ToLower(entity.Type)+":"+domain.CanonicalValue(entity.Type, entity.Value))
	}
	sort.Strings(entityKeys)
	// Structured encoding distinguishes null from a literal "<null>" group value.
	raw, _ := json.Marshal(struct {
		Sources      []string
		From, To     time.Time
		Entities     []string
		Filter       string
		Columns      []string
		Sort         []capability.EventSort
		GroupBy      []string
		GroupValues  []*string
		CommonFilter fieldmap.CommonFilter
		Language     string
		Mapping      string
	}{sources, request.TimeFrom.UTC(), request.TimeTo.UTC(), entityKeys, request.Filter, request.Columns, request.Sort, request.GroupBy, request.GroupValues, request.CommonFilter, request.FilterLanguage, fieldmap.Hash()})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func compareEvents(left, right domain.Event, rules []capability.EventSort) int {
	for _, rule := range rules {
		a, b := eventFieldValue(left, rule.Field), eventFieldValue(right, rule.Field)
		if a == nil && b != nil {
			return 1
		}
		if a != nil && b == nil {
			return -1
		}
		cmp := compareEventField(left, right, rule.Field)
		if cmp != 0 {
			if rule.Direction == "desc" {
				return -cmp
			}
			return cmp
		}
	}
	if cmp := left.OccurredAt.Compare(right.OccurredAt); cmp != 0 {
		return -cmp
	}
	if cmp := strings.Compare(left.Provenance.Source, right.Provenance.Source); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.Provenance.ExternalID, right.Provenance.ExternalID)
}
func compareEventField(left, right domain.Event, field string) int {
	if field == "time" || field == "timestamp" || field == "@timestamp" {
		return left.OccurredAt.Compare(right.OccurredAt)
	}
	a, b := eventFieldValue(left, field), eventFieldValue(right, field)
	x, xok := numericEventValue(a)
	y, yok := numericEventValue(b)
	if xok && yok {
		return x.Cmp(y)
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}
func numericEventValue(value any) (*big.Rat, bool) {
	switch value.(type) {
	case json.Number, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8, float64, float32:
		return new(big.Rat).SetString(fmt.Sprint(value))
	default:
		return nil, false
	}
}
func eventFieldValue(event domain.Event, field string) any {
	switch field {
	case "time", "timestamp", "@timestamp":
		return event.OccurredAt
	case "text":
		return event.Title
	case "importance":
		return event.Severity
	case "uuid":
		return event.Provenance.ExternalID
	default:
		return event.Attributes[field]
	}
}
