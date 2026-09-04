package maxpatrol

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const SourceCode = "pt-maxpatrol-siem"

func ToEventPage(response EventsResponse, offset int, fetchedAt time.Time) (capability.EventPage, error) {
	if response.TotalCount < 0 || offset < 0 {
		return capability.EventPage{}, fmt.Errorf("invalid MaxPatrol pagination")
	}
	if int64(offset+len(response.Events)) > response.TotalCount {
		return capability.EventPage{}, fmt.Errorf("MaxPatrol page exceeds totalCount")
	}
	page := capability.EventPage{
		Events: make([]domain.Event, 0, len(response.Events)),
		Status: "complete",
		Total:  authenticMatchTotal(response.TotalCount, len(response.Events)),
	}
	if int64(offset+len(response.Events)) < response.TotalCount {
		// The reviewed capture does not establish how response.Token is
		// exchanged for a following page, so it must not become a cursor.
		page.Status = "truncated"
	}
	entities := make(map[string]domain.Entity)
	for index, record := range response.Events {
		event, eventEntities, err := toEvent(record, fetchedAt)
		if err != nil {
			return capability.EventPage{}, fmt.Errorf("map MaxPatrol event %d: %w", index, err)
		}
		page.Events = append(page.Events, event)
		for _, entity := range eventEntities {
			entities[entity.Type+"\x00"+entity.Value] = entity
		}
	}
	for _, entity := range entities {
		page.Entities = append(page.Entities, entity)
	}
	sort.Slice(page.Entities, func(i, j int) bool {
		if page.Entities[i].Type != page.Entities[j].Type {
			return page.Entities[i].Type < page.Entities[j].Type
		}
		return page.Entities[i].Value < page.Entities[j].Value
	})
	return page, nil
}

func toEvent(record EventRecord, fetchedAt time.Time) (domain.Event, []domain.Entity, error) {
	externalID := strings.TrimSpace(record.UUID)
	if externalID == "" {
		return domain.Event{}, nil, fmt.Errorf("uuid is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, record.Time)
	if err != nil {
		return domain.Event{}, nil, fmt.Errorf("invalid time: %w", err)
	}
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Text) == "" {
		return domain.Event{}, nil, fmt.Errorf("id and text are required")
	}

	provenance := domain.Provenance{Source: SourceCode, ExternalID: externalID, FetchedAt: fetchedAt}
	attributes := map[string]any{"vendor_event_id": record.ID}
	putString(attributes, "event_src.host", record.EventSourceHost)
	putString(attributes, "event_src.ip", record.EventSourceIP)
	putString(attributes, "src.ip", record.SourceIP)
	putString(attributes, "dst.ip", record.DestinationIP)
	if record.DestinationPort > 0 {
		attributes["dst.port"] = record.DestinationPort
	}
	if record.CorrelationName != nil && strings.TrimSpace(*record.CorrelationName) != "" {
		attributes["correlation_name"] = strings.TrimSpace(*record.CorrelationName)
	}
	event := domain.Event{
		Type:       normalize(record.ID),
		Title:      strings.TrimSpace(record.Text),
		Severity:   severity(record.Importance),
		OccurredAt: occurredAt.UTC(),
		Attributes: attributes,
		Provenance: provenance,
	}
	entities := make([]domain.Entity, 0, 4)
	seenEntities := make(map[string]struct{}, 4)
	addEntity := func(kind, value, role string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		entityProvenance := domain.Provenance{
			Source:     SourceCode,
			ExternalID: kind + ":" + domain.CanonicalValue(kind, value),
			FetchedAt:  fetchedAt,
		}
		entity := domain.NewEntity(kind, value, entityProvenance)
		key := entity.Type + "\x00" + entity.Value
		if _, exists := seenEntities[key]; exists {
			return
		}
		seenEntities[key] = struct{}{}
		entities = append(entities, entity)
		event.Entities = append(event.Entities, domain.EntityMention{
			EntityRef: domain.EntityRef{Type: entity.Type, Value: entity.Value},
			Roles:     []string{role},
		})
	}
	addEntity("host", record.EventSourceHost, "mentions")
	addEntity("ip", record.EventSourceIP, "mentions")
	addEntity("ip", record.SourceIP, "src")
	addEntity("ip", record.DestinationIP, "dst")
	sort.Slice(event.Entities, func(i, j int) bool {
		if event.Entities[i].Type != event.Entities[j].Type {
			return event.Entities[i].Type < event.Entities[j].Type
		}
		return event.Entities[i].Value < event.Entities[j].Value
	})
	return event, entities, nil
}

func putString(target map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		target[key] = value
	}
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "_", "-", "_").Replace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func severity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}
