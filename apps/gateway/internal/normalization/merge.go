package normalization

import (
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func Events(items []domain.Event) []domain.Event {
	seen := make(map[string]domain.Event, len(items))
	for _, item := range items {
		key := item.Provenance.Source + "\x00" + item.Provenance.ExternalID
		seen[key] = item
	}
	result := make([]domain.Event, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].OccurredAt.Equal(result[j].OccurredAt) {
			return result[i].OccurredAt.After(result[j].OccurredAt)
		}
		if result[i].Provenance.Source != result[j].Provenance.Source {
			return result[i].Provenance.Source < result[j].Provenance.Source
		}
		return result[i].Provenance.ExternalID < result[j].Provenance.ExternalID
	})
	return result
}

func Entities(items []domain.Entity) []domain.Entity {
	seen := make(map[string]domain.Entity, len(items))
	for _, item := range items {
		key := item.Type + "\x00" + domain.CanonicalValue(item.Type, item.Value)
		current, exists := seen[key]
		if !exists {
			seen[key] = item
			continue
		}
		current.Provenance = appendUniqueProvenance(current.Provenance, item.Provenance...)
		for attribute, value := range item.Attributes {
			if _, exists := current.Attributes[attribute]; !exists {
				current.Attributes[attribute] = value
			}
		}
		seen[key] = current
	}
	result := make([]domain.Entity, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Value < result[j].Value
	})
	return result
}

func Relations(items []domain.Relation) []domain.Relation {
	seen := make(map[string]domain.Relation, len(items))
	for _, item := range items {
		key := item.Provenance.Source + "\x00" + item.Provenance.ExternalID
		seen[key] = item
	}
	result := make([]domain.Relation, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provenance.Source != result[j].Provenance.Source {
			return result[i].Provenance.Source < result[j].Provenance.Source
		}
		return result[i].Provenance.ExternalID < result[j].Provenance.ExternalID
	})
	return result
}

func Endpoints(items []domain.Endpoint) []domain.Endpoint {
	seen := make(map[string]domain.Endpoint, len(items))
	for _, item := range items {
		key := item.Provenance.Source + "\x00" + item.ExternalID
		seen[key] = item
	}
	result := make([]domain.Endpoint, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Hostname != result[j].Hostname {
			return result[i].Hostname < result[j].Hostname
		}
		if result[i].Provenance.Source != result[j].Provenance.Source {
			return result[i].Provenance.Source < result[j].Provenance.Source
		}
		return result[i].ExternalID < result[j].ExternalID
	})
	return result
}

func appendUniqueProvenance(items []domain.Provenance, additions ...domain.Provenance) []domain.Provenance {
	seen := make(map[string]struct{}, len(items)+len(additions))
	for _, item := range items {
		seen[item.Source+"\x00"+item.ExternalID] = struct{}{}
	}
	for _, item := range additions {
		key := item.Source + "\x00" + item.ExternalID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	return items
}
