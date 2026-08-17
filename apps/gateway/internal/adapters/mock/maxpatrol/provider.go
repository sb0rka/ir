package maxpatrol

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	maxpatrolapi "github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const SourceCode = maxpatrolapi.SourceCode

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type mock struct {
	scenario scenario.Scenario
}

func NewMock(value scenario.Scenario) registry.Provider {
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "MaxPatrol SIEM",
			Kind:         "siem",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEvents},
		},
		Events: &mock{scenario: value},
	}
}

func (adapter *mock) SearchEvents(ctx context.Context, request capability.SearchEventsRequest) (capability.EventPage, error) {
	if err := ctx.Err(); err != nil {
		return capability.EventPage{}, err
	}
	token, offset, err := parseCursor(request.Cursor)
	if err != nil {
		return capability.EventPage{}, &domain.RequestError{Code: "invalid_cursor", Message: err.Error()}
	}
	limit := request.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	vendorRequest := toEventsRequest(request)
	wantedEntities := entitySet(request.Entities)

	matched := make([]scenario.Event, 0)
	for index, event := range adapter.scenario.EventsForSource("MaxPatrol") {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return capability.EventPage{}, err
			}
		}
		if matchesEvent(adapter.scenario, event, vendorRequest, request.Query, wantedEntities) {
			matched = append(matched, event)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		left, right := scenario.ParseTime(matched[i].EventTS), scenario.ParseTime(matched[j].EventTS)
		if !left.Equal(right) {
			return left.After(right)
		}
		return matched[i].ID < matched[j].ID
	})
	if offset > len(matched) {
		return capability.EventPage{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is past the result set"}
	}
	expectedToken := domain.StableID("maxpatrol-events-token", requestFingerprint(request)).String()
	if token != "" && token != expectedToken {
		return capability.EventPage{}, &domain.RequestError{Code: "invalid_cursor", Message: "source cursor token does not match the request"}
	}
	end := min(offset+limit, len(matched))
	response := maxpatrolapi.EventsResponse{
		Token:      expectedToken,
		TotalCount: int64(len(matched)),
		Events:     make([]maxpatrolapi.EventRecord, 0, end-offset),
	}
	for _, event := range matched[offset:end] {
		response.Events = append(response.Events, adapter.vendorEvent(event))
	}
	return maxpatrolapi.ToEventPage(response, offset, fetchedAt)
}

func (adapter *mock) ResolveContext(ctx context.Context, request capability.ResolveContextRequest) (capability.EventPage, error) {
	wantedEvents := stringSet(request.EventIDs)
	wantedEntities := stringSet(request.EntityIDs)
	foundEvents := make(map[string]struct{}, len(wantedEvents))
	foundEntities := make(map[string]struct{}, len(wantedEntities))
	result := capability.EventPage{}
	entities := make([]domain.Entity, 0)

	for index, event := range adapter.scenario.EventsForSource("MaxPatrol") {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return capability.EventPage{}, err
			}
		}
		page, err := maxpatrolapi.ToEventPage(maxpatrolapi.EventsResponse{
			Token: "resolve", TotalCount: 1, Events: []maxpatrolapi.EventRecord{adapter.vendorEvent(event)},
		}, 0, fetchedAt)
		if err != nil {
			return capability.EventPage{}, err
		}
		selectedEvent := false
		if len(page.Events) == 1 {
			_, selectedEvent = wantedEvents[page.Events[0].Provenance.ExternalID]
			if selectedEvent {
				foundEvents[page.Events[0].Provenance.ExternalID] = struct{}{}
				result.Events = append(result.Events, page.Events[0])
			}
		}
		for _, entity := range page.Entities {
			selectedEntity := selectedEvent
			for _, provenance := range entity.Provenance {
				if _, wanted := wantedEntities[provenance.ExternalID]; wanted {
					selectedEntity = true
					foundEntities[provenance.ExternalID] = struct{}{}
				}
			}
			if selectedEntity {
				entities = append(entities, entity)
			}
		}
	}
	for _, node := range adapter.scenario.NodesForSystem("MaxPatrol") {
		entity, ok := adapter.scenario.EntityForNode(node, SourceCode, fetchedAt)
		if !ok {
			continue
		}
		if _, wanted := wantedEntities[node.ID]; wanted {
			foundEntities[node.ID] = struct{}{}
			entities = append(entities, entity)
		}
	}
	if missing := missingIDs(wantedEvents, foundEvents); len(missing) > 0 {
		return capability.EventPage{}, &domain.RequestError{Code: "source_record_not_found", Message: fmt.Sprintf("events not found: %s", strings.Join(missing, ", "))}
	}
	if missing := missingIDs(wantedEntities, foundEntities); len(missing) > 0 {
		return capability.EventPage{}, &domain.RequestError{Code: "source_record_not_found", Message: fmt.Sprintf("entities not found: %s", strings.Join(missing, ", "))}
	}
	result.Entities = normalization.Entities(entities)
	return result, nil
}

func toEventsRequest(request capability.SearchEventsRequest) maxpatrolapi.EventsRequest {
	top := int64(request.Limit)
	if top < 1 || top > 100 {
		top = 50
	}
	return maxpatrolapi.EventsRequest{
		Filter: maxpatrolapi.EventsFilter{
			GroupBy: []string{},
			OrderBy: []maxpatrolapi.OrderBy{{Field: "time", SortOrder: "descending"}},
			Select: []string{
				"time", "uuid", "id", "text", "importance", "event_src.host",
				"event_src.ip", "src.ip", "dst.ip", "dst.port", "correlation_name",
			},
			Top: &top,
		},
		GroupValues: []string{},
		TimeFrom:    unixOrZero(request.TimeFrom),
		TimeTo:      unixOrZero(request.TimeTo),
	}
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func (adapter *mock) vendorEvent(event scenario.Event) maxpatrolapi.EventRecord {
	record := maxpatrolapi.EventRecord{
		Time:       event.EventTS,
		UUID:       event.ID,
		ID:         event.EventClass,
		Text:       event.Title,
		Importance: event.Severity,
	}
	if strings.Contains(strings.ToLower(event.EventClass), "correlation") {
		name := "impacket_smbexec"
		record.CorrelationName = &name
	}
	for index, nodeID := range event.EntityIDs {
		node, ok := adapter.scenario.Node(nodeID)
		if !ok {
			continue
		}
		hostname := strings.TrimSpace(strings.Split(node.Data.Label, "\n")[0])
		ip, _ := node.Data.Details["ip"].(string)
		if index == 0 {
			record.EventSourceHost = hostname
			record.EventSourceIP = ip
			record.SourceIP = ip
		} else if record.DestinationIP == "" {
			record.DestinationIP = ip
		}
	}
	return record
}

func matchesEvent(value scenario.Scenario, event scenario.Event, request maxpatrolapi.EventsRequest, query string, entities map[string]struct{}) bool {
	timestamp := scenario.ParseTime(event.EventTS)
	if request.TimeFrom != 0 && timestamp.Unix() < request.TimeFrom ||
		request.TimeTo != 0 && timestamp.Unix() > request.TimeTo {
		return false
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" && !strings.Contains(strings.ToLower(event.Title+" "+event.EventClass+" "+event.Severity), query) {
		return false
	}
	if len(entities) == 0 {
		return true
	}
	for _, nodeID := range event.EntityIDs {
		node, ok := value.Node(nodeID)
		if !ok {
			continue
		}
		entity, ok := value.EntityForNode(node, "filter", fetchedAt)
		if !ok {
			continue
		}
		if _, wanted := entities[entity.Type+"\x00"+entity.Value]; wanted {
			return true
		}
	}
	return false
}

func entitySet(entities []domain.EntityRef) map[string]struct{} {
	result := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		kind := strings.ToLower(strings.TrimSpace(entity.Type))
		result[kind+"\x00"+domain.CanonicalValue(kind, entity.Value)] = struct{}{}
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[strings.TrimSpace(value)] = struct{}{}
	}
	return result
}

func missingIDs(wanted, found map[string]struct{}) []string {
	result := make([]string, 0)
	for id := range wanted {
		if _, ok := found[id]; !ok {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func parseCursor(raw string) (string, int, error) {
	if strings.TrimSpace(raw) == "" {
		return "", 0, nil
	}
	separator := strings.LastIndex(raw, ":")
	if separator <= 0 || separator == len(raw)-1 {
		return "", 0, fmt.Errorf("source cursor is invalid")
	}
	offset, err := strconv.Atoi(raw[separator+1:])
	if err != nil || offset < 0 {
		return "", 0, fmt.Errorf("source cursor is invalid")
	}
	return raw[:separator], offset, nil
}

func requestFingerprint(request capability.SearchEventsRequest) string {
	parts := []string{request.TimeFrom.UTC().Format(time.RFC3339Nano), request.TimeTo.UTC().Format(time.RFC3339Nano), strings.TrimSpace(request.Query)}
	entityParts := make([]string, 0, len(request.Entities))
	for _, entity := range request.Entities {
		entityParts = append(entityParts, strings.ToLower(strings.TrimSpace(entity.Type))+":"+domain.CanonicalValue(entity.Type, entity.Value))
	}
	sort.Strings(entityParts)
	parts = append(parts, entityParts...)
	return strings.Join(parts, "\x00")
}
