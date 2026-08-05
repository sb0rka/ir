package eventmock

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
)

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type EnrichFunc func(scenario.Event, map[string]any)

type Adapter struct {
	scenario      scenario.Scenario
	fixtureSource string
	sourceCode    string
	enrich        EnrichFunc
}

func New(value scenario.Scenario, fixtureSource, sourceCode string, enrich EnrichFunc) *Adapter {
	return &Adapter{scenario: value, fixtureSource: fixtureSource, sourceCode: sourceCode, enrich: enrich}
}

func (adapter *Adapter) SearchEvents(ctx context.Context, request capability.SearchEventsRequest) (capability.EventPage, error) {
	if err := ctx.Err(); err != nil {
		return capability.EventPage{}, err
	}
	offset, err := parseCursor(request.Cursor)
	if err != nil {
		return capability.EventPage{}, &domain.RequestError{Code: "invalid_cursor", Message: err.Error()}
	}
	limit := request.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	matched := make([]scenario.Event, 0)
	for _, event := range adapter.scenario.EventsForSource(adapter.fixtureSource) {
		if !matchesEvent(adapter.scenario, event, request) {
			continue
		}
		matched = append(matched, event)
	}
	sort.Slice(matched, func(i, j int) bool {
		left, right := scenario.ParseTime(matched[i].EventTS), scenario.ParseTime(matched[j].EventTS)
		if !left.Equal(right) {
			return left.After(right)
		}
		return matched[i].ID < matched[j].ID
	})
	total := len(matched)
	if offset > total {
		return capability.EventPage{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is past the result set"}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	matched = matched[offset:end]

	page := capability.EventPage{
		Events:        make([]domain.Event, 0, len(matched)),
		Continuations: make([]string, 0, len(matched)),
		HasMore:       end < total,
	}
	nodeEntities := make(map[string]domain.Entity)
	for index, item := range matched {
		occurredAt := scenario.ParseTime(item.EventTS)
		provenance := domain.Provenance{
			Source:     adapter.sourceCode,
			ExternalID: item.ID,
			FetchedAt:  fetchedAt,
		}
		attributes := map[string]any{"event_class": item.EventClass}
		if adapter.enrich != nil {
			adapter.enrich(item, attributes)
		}
		event := domain.Event{
			ID:         domain.StableID("event", adapter.sourceCode, item.ID),
			Type:       normalizeType(item.EventClass),
			Title:      item.Title,
			Severity:   normalizeSeverity(item.Severity),
			OccurredAt: occurredAt,
			Attributes: attributes,
			Provenance: provenance,
		}
		for _, nodeID := range item.EntityIDs {
			node, ok := adapter.scenario.Node(nodeID)
			if !ok {
				continue
			}
			entity, ok := adapter.scenario.EntityForNode(node, adapter.sourceCode, fetchedAt)
			if !ok {
				continue
			}
			nodeEntities[nodeID] = entity
			event.EntityIDs = append(event.EntityIDs, entity.ID)
		}
		page.Events = append(page.Events, event)
		page.Continuations = append(page.Continuations, strconv.Itoa(offset+index+1))
	}

	for _, entity := range nodeEntities {
		page.Entities = append(page.Entities, entity)
	}
	for _, edge := range adapter.scenario.Edges {
		source, sourceOK := nodeEntities[edge.Source]
		target, targetOK := nodeEntities[edge.Target]
		if !sourceOK || !targetOK {
			continue
		}
		provenance := domain.Provenance{
			Source:     adapter.sourceCode,
			ExternalID: edge.ID,
			FetchedAt:  fetchedAt,
		}
		page.Relations = append(page.Relations, domain.Relation{
			ID:             domain.StableID("relation", adapter.sourceCode, edge.ID),
			Type:           normalizeType(edge.Label),
			SourceEntityID: source.ID,
			TargetEntityID: target.ID,
			Provenance:     provenance,
		})
	}
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	return page, nil
}

func matchesEvent(value scenario.Scenario, event scenario.Event, request capability.SearchEventsRequest) bool {
	timestamp := scenario.ParseTime(event.EventTS)
	if !request.TimeFrom.IsZero() && timestamp.Before(request.TimeFrom) {
		return false
	}
	if !request.TimeTo.IsZero() && timestamp.After(request.TimeTo) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	if query != "" && !strings.Contains(strings.ToLower(event.Title+" "+event.EventClass+" "+event.Severity), query) {
		return false
	}
	if len(request.Entities) == 0 {
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
		for _, wanted := range request.Entities {
			if strings.EqualFold(entity.Type, wanted.Type) && entity.Value == domain.CanonicalValue(wanted.Type, wanted.Value) {
				return true
			}
		}
	}
	return false
}

func parseCursor(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("source cursor is invalid")
	}
	return value, nil
}

func normalizeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "_", "-", "_").Replace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}
