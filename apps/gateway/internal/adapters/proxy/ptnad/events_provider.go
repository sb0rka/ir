package ptnad

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

func (provider *Provider) SearchEvents(ctx context.Context, access capability.Access, request capability.SearchEventsRequest) (capability.EventPage, error) {
	if strings.TrimSpace(request.Cursor) != "" {
		return capability.EventPage{}, invalidRequest("PT NAD does not expose a confirmed event cursor")
	}
	if hasSourceEventControls(request) {
		return capability.EventPage{}, invalidRequest("PT NAD does not support filter, columns, sort, or grouping")
	}
	timeRange := domain.TimeRange{From: request.TimeFrom, To: request.TimeTo}
	if err := validateDomainTimeRange(timeRange); err != nil {
		return capability.EventPage{}, err
	}
	limit, err := validateCapabilityLimit(request.Limit)
	if err != nil {
		return capability.EventPage{}, err
	}
	vendorLimit := limit
	if len(request.Entities) > 0 {
		vendorLimit = MaxLimit
	}
	sessionResults, sessionPartial, sessionErr := fanOutStores(ctx, provider, func(storeID int64) (SessionSearchResult, error) {
		return provider.client.SearchSessions(ctx, SearchRequest{
			StoreID: storeID, From: request.TimeFrom, To: request.TimeTo, Limit: vendorLimit,
		}, Access{Cookie: access.Cookie})
	})
	attackResults, attackPartial, attackErr := fanOutStores(ctx, provider, func(storeID int64) (AttackSearchResult, error) {
		return provider.client.SearchAttacks(ctx, SearchRequest{
			StoreID: storeID, From: request.TimeFrom, To: request.TimeTo, Limit: vendorLimit,
		}, Access{Cookie: access.Cookie})
	})
	if sessionErr != nil && attackErr != nil {
		return capability.EventPage{}, canonicalProviderError(sessionErr)
	}
	partial := sessionPartial || attackPartial || sessionErr != nil || attackErr != nil
	page := capability.EventPage{Status: "complete"}
	var total int64
	totalsComplete := sessionErr == nil && attackErr == nil
	for _, result := range sessionResults {
		partial = partial || result.Truncated
		total += result.Total
		for _, session := range result.Sessions {
			event := sessionEvent(session)
			if !eventMatchesEntities(event, request.Entities) {
				continue
			}
			page.Events = append(page.Events, event)
			page.Entities = append(page.Entities, entitiesForEvent(event, session.SourceRef.SourceInstance)...)
			page.Relations = append(page.Relations, sessionRelations(session)...)
		}
	}
	for _, result := range attackResults {
		partial = partial || result.Truncated
		total += result.Total
		for _, attack := range result.Attacks {
			event := attackEvent(attack)
			if !eventMatchesEntities(event, request.Entities) {
				continue
			}
			page.Events = append(page.Events, event)
			page.Entities = append(page.Entities, entitiesForEvent(event, attack.SourceRef.SourceInstance)...)
		}
	}
	// Entity filters drop rows after the vendor counted them, so the sum is not a match total.
	if totalsComplete && len(request.Entities) == 0 {
		page.Total = &total
	}
	page.Events = normalization.Events(page.Events)
	if len(page.Events) > limit {
		page.Events = page.Events[:limit]
		partial = true
	}
	selected := make(map[string]struct{})
	for _, event := range page.Events {
		for _, mention := range event.Entities {
			selected[entityIdentity(mention.EntityRef)] = struct{}{}
		}
	}
	page.Entities = filterCanonicalEntities(normalization.Entities(page.Entities), selected)
	page.Relations = filterCanonicalRelations(normalization.Relations(page.Relations), selected)
	if partial {
		page.Status = "truncated"
	}
	return page, nil
}

func hasSourceEventControls(request capability.SearchEventsRequest) bool {
	return strings.TrimSpace(request.Filter) != "" || len(request.Columns) > 0 || len(request.Sort) > 0 ||
		len(request.GroupBy) > 0 || len(request.GroupValues) > 0
}

func (provider *Provider) ResolveContext(ctx context.Context, access capability.Access, request capability.ResolveContextRequest) (capability.ContextPage, error) {
	page := capability.ContextPage{}
	storeRanges := make(map[int64]TimeRange)
	getStoreRange := func(storeID int64) (TimeRange, error) {
		if value, exists := storeRanges[storeID]; exists {
			return value, nil
		}
		store, err := provider.client.GetStore(ctx, storeID, Access{Cookie: access.Cookie})
		if err != nil {
			return TimeRange{}, err
		}
		if !store.Start.Before(store.End) {
			return TimeRange{}, &ProtocolError{Operation: "store detail"}
		}
		value := TimeRange{From: store.Start, To: store.End}
		storeRanges[storeID] = value
		return value, nil
	}

	wantedEvents := make(map[string]struct{}, len(request.EventIDs))
	for _, sourceEventID := range request.EventIDs {
		sourceEventID = strings.TrimSpace(sourceEventID)
		wantedEvents[sourceEventID] = struct{}{}
		parsed, err := provider.parseEventID(sourceEventID)
		if err != nil {
			return capability.ContextPage{}, err
		}
		timeRange, err := getStoreRange(parsed.StoreID)
		if err != nil {
			return capability.ContextPage{}, canonicalProviderError(err)
		}
		switch parsed.RecordType {
		case SessionRecordType:
			session, getErr := provider.client.GetSession(ctx, SessionRef{
				StoreID: parsed.StoreID, ExternalID: parsed.ExternalID, TimeRange: timeRange,
			}, Access{Cookie: access.Cookie})
			if getErr != nil {
				return capability.ContextPage{}, canonicalProviderError(getErr)
			}
			appendSessionContext(&page, session)
		case AttackRecordType:
			attack, getErr := provider.client.GetAttack(ctx, AttackRef{
				StoreID: parsed.StoreID, ExternalID: parsed.ExternalID, TimeRange: timeRange,
			}, Access{Cookie: access.Cookie})
			if getErr != nil {
				return capability.ContextPage{}, canonicalProviderError(getErr)
			}
			if attack.ParentSession == nil {
				appendAttackContext(&page, attack)
				break
			}
			session, getErr := provider.client.GetSession(ctx, SessionRef{
				StoreID: parsed.StoreID, ExternalID: attack.ParentSession.ExternalID, TimeRange: timeRange,
			}, Access{Cookie: access.Cookie})
			if getErr != nil {
				appendAttackContext(&page, attack)
				break
			}
			appendSessionContext(&page, session)
		default:
			if parsed.ParentID == "" {
				return capability.ContextPage{}, invalidRequest("PT NAD source event ID is invalid")
			}
			session, getErr := provider.client.GetSession(ctx, SessionRef{
				StoreID: parsed.StoreID, ExternalID: parsed.ParentID, TimeRange: timeRange,
			}, Access{Cookie: access.Cookie})
			if getErr != nil {
				return capability.ContextPage{}, canonicalProviderError(getErr)
			}
			appendSessionContext(&page, session)
		}
	}

	for _, sourceEntityID := range request.EntityIDs {
		storeID, entity, err := provider.parseEntityID(sourceEntityID)
		if err != nil {
			return capability.ContextPage{}, err
		}
		timeRange, err := getStoreRange(storeID)
		if err != nil {
			return capability.ContextPage{}, canonicalProviderError(err)
		}
		lookup, err := provider.LookupEntity(ctx, access, capability.LookupEntityRequest{
			Entity: entity, TimeRange: domain.TimeRange{From: timeRange.From, To: timeRange.To},
		})
		if err != nil {
			return capability.ContextPage{}, err
		}
		found := false
		for _, candidate := range lookup.Entities {
			for _, provenance := range candidate.Provenance {
				if provenance.ExternalID == sourceEntityID {
					found = true
				}
			}
		}
		if !found {
			return capability.ContextPage{}, &domain.RequestError{Code: "source_record_not_found", Message: "PT NAD entity was not found"}
		}
		page.Entities = append(page.Entities, lookup.Entities...)
		page.Relations = append(page.Relations, lookup.Relations...)
	}

	page = normalizeContextPage(page)
	if len(wantedEvents) > 0 {
		found := make(map[string]struct{}, len(page.Events))
		for _, event := range page.Events {
			found[event.Provenance.ExternalID] = struct{}{}
		}
		for eventID := range wantedEvents {
			if _, exists := found[eventID]; !exists {
				return capability.ContextPage{}, &domain.RequestError{Code: "source_record_not_found", Message: "PT NAD event was not found"}
			}
		}
	}
	return page, nil
}

func (provider *Provider) LookupEntity(ctx context.Context, access capability.Access, request capability.LookupEntityRequest) (capability.LookupEntityResult, error) {
	if err := validateDomainTimeRange(request.TimeRange); err != nil {
		return capability.LookupEntityResult{}, err
	}
	entity := entityRef(request.Entity.Type, request.Entity.Value)
	if entity.Type == "" || entity.Value == "" {
		return capability.LookupEntityResult{}, invalidRequest("PT NAD entity is invalid")
	}
	page, err := provider.SearchEvents(ctx, access, capability.SearchEventsRequest{
		TimeFrom: request.TimeRange.From, TimeTo: request.TimeRange.To,
		Entities: []domain.EntityRef{entity}, Limit: MaxLimit,
	})
	if err != nil {
		return capability.LookupEntityResult{}, err
	}
	return capability.LookupEntityResult{
		Entities: page.Entities, Relations: page.Relations, Verdicts: []domain.Verdict{},
	}, nil
}

func eventMatchesEntities(event domain.Event, wanted []domain.EntityRef) bool {
	if len(wanted) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(wanted))
	for _, entity := range wanted {
		set[entityIdentity(entityRef(entity.Type, entity.Value))] = struct{}{}
	}
	for _, mention := range event.Entities {
		if _, exists := set[entityIdentity(mention.EntityRef)]; exists {
			return true
		}
	}
	return false
}

func entityIdentity(value domain.EntityRef) string {
	value = entityRef(value.Type, value.Value)
	return value.Type + "\x00" + value.Value
}

func filterCanonicalEntities(values []domain.Entity, selected map[string]struct{}) []domain.Entity {
	result := make([]domain.Entity, 0, len(values))
	for _, value := range values {
		if _, exists := selected[entityIdentity(domain.EntityRef{Type: value.Type, Value: value.Value})]; exists {
			result = append(result, value)
		}
	}
	return result
}

func filterCanonicalRelations(values []domain.Relation, selected map[string]struct{}) []domain.Relation {
	result := make([]domain.Relation, 0, len(values))
	for _, value := range values {
		_, sourceExists := selected[entityIdentity(value.SourceEntity)]
		_, targetExists := selected[entityIdentity(value.TargetEntity)]
		if sourceExists && targetExists {
			result = append(result, value)
		}
	}
	return result
}

type parsedEventID struct {
	StoreID    int64
	RecordType string
	ParentID   string
	ExternalID string
}

func (provider *Provider) parseEventID(value string) (parsedEventID, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 && len(parts) != 4 {
		return parsedEventID{}, invalidRequest("PT NAD source event ID is invalid")
	}
	storeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || strconv.FormatInt(storeID, 10) != parts[0] {
		return parsedEventID{}, invalidRequest("PT NAD source event ID is invalid")
	}
	if _, configured := provider.stores[storeID]; !configured {
		return parsedEventID{}, invalidRequest("PT NAD source event store is not configured")
	}
	result := parsedEventID{StoreID: storeID, RecordType: parts[1]}
	if len(parts) == 3 {
		result.ExternalID = parts[2]
	} else {
		result.ParentID = parts[2]
		result.ExternalID = parts[3]
	}
	if err := validateExternalID(result.ExternalID); err != nil {
		return parsedEventID{}, invalidRequest("PT NAD source event ID is invalid")
	}
	if result.ParentID != "" {
		if err := validateExternalID(result.ParentID); err != nil {
			return parsedEventID{}, invalidRequest("PT NAD source event ID is invalid")
		}
	}
	return result, nil
}

func entitySourceID(storeID, kind, value string) string {
	return storeID + ":entity:" + strings.ToLower(strings.TrimSpace(kind)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(domain.CanonicalValue(kind, value)))
}

func (provider *Provider) parseEntityID(value string) (int64, domain.EntityRef, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[1] != "entity" {
		return 0, domain.EntityRef{}, invalidRequest("PT NAD source entity ID is invalid")
	}
	storeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || strconv.FormatInt(storeID, 10) != parts[0] {
		return 0, domain.EntityRef{}, invalidRequest("PT NAD source entity ID is invalid")
	}
	if _, configured := provider.stores[storeID]; !configured {
		return 0, domain.EntityRef{}, invalidRequest("PT NAD source entity store is not configured")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(decoded) == 0 {
		return 0, domain.EntityRef{}, invalidRequest("PT NAD source entity ID is invalid")
	}
	return storeID, entityRef(parts[2], string(decoded)), nil
}
