package maxpatrol

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const SourceCode = "maxpatrol-siem"

func ToEventPage(response EventsResponse, offset int, fetchedAt time.Time) (capability.EventPage, error) {
	if response.TotalCount < 0 || offset < 0 {
		return capability.EventPage{}, fmt.Errorf("invalid MaxPatrol pagination")
	}
	if int64(offset+len(response.Events)) > response.TotalCount {
		return capability.EventPage{}, fmt.Errorf("MaxPatrol page exceeds totalCount")
	}
	if len(response.Events) > 0 && strings.TrimSpace(response.Token) == "" {
		return capability.EventPage{}, fmt.Errorf("MaxPatrol response has events but no token")
	}

	page := capability.EventPage{
		Events:        make([]domain.Event, 0, len(response.Events)),
		Continuations: make([]string, 0, len(response.Events)),
		HasMore:       int64(offset+len(response.Events)) < response.TotalCount,
	}
	entities := make(map[string]domain.Entity)
	for index, record := range response.Events {
		event, eventEntities, err := toEvent(record, fetchedAt)
		if err != nil {
			return capability.EventPage{}, fmt.Errorf("map MaxPatrol event %d: %w", index, err)
		}
		page.Events = append(page.Events, event)
		page.Continuations = append(page.Continuations, response.Token+":"+strconv.Itoa(offset+index+1))
		for _, entity := range eventEntities {
			entities[entity.ID.String()] = entity
		}
	}
	for _, entity := range entities {
		page.Entities = append(page.Entities, entity)
	}
	sort.Slice(page.Entities, func(i, j int) bool { return page.Entities[i].ID.String() < page.Entities[j].ID.String() })
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
	if len(record.Fields) > 0 {
		fields := make(map[string]any, len(record.Fields))
		for key, raw := range record.Fields {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return domain.Event{}, nil, fmt.Errorf("decode dynamic field %q: %w", key, err)
			}
			fields[key] = value
		}
		attributes["vendor_fields"] = fields
	}

	event := domain.Event{
		ID:         domain.StableID("event", SourceCode, externalID),
		Type:       normalize(record.ID),
		Title:      strings.TrimSpace(record.Text),
		Severity:   severity(record.Importance),
		OccurredAt: occurredAt.UTC(),
		Attributes: attributes,
		Provenance: provenance,
	}
	entities := make([]domain.Entity, 0, 4)
	seenEntities := make(map[string]struct{}, 4)
	addEntity := func(kind, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		entityProvenance := domain.Provenance{
			Source:     SourceCode,
			ExternalID: kind + ":" + domain.CanonicalValue(kind, value),
			FetchedAt:  fetchedAt,
		}
		entity := domain.NewEntity(kind, value, entityProvenance)
		if _, exists := seenEntities[entity.ID.String()]; exists {
			return
		}
		seenEntities[entity.ID.String()] = struct{}{}
		entities = append(entities, entity)
		event.EntityIDs = append(event.EntityIDs, entity.ID)
	}
	addEntity("host", record.EventSourceHost)
	addEntity("ip", record.EventSourceIP)
	addEntity("ip", record.SourceIP)
	addEntity("ip", record.DestinationIP)
	sort.Slice(event.EntityIDs, func(i, j int) bool { return event.EntityIDs[i].String() < event.EntityIDs[j].String() })
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
