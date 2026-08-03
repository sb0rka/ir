package normalization

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestEntitiesDeduplicateCanonicalValue(t *testing.T) {
	first := domain.NewEntity("domain", "Example.COM", domain.Provenance{Source: "one", ExternalID: "1"})
	second := domain.NewEntity("domain", "example.com", domain.Provenance{Source: "two", ExternalID: "2"})
	second.Attributes["score"] = 90

	result := Entities([]domain.Entity{first, second})
	if len(result) != 1 {
		t.Fatalf("got %d entities, want 1", len(result))
	}
	if len(result[0].Provenance) != 2 || result[0].Attributes["score"] != 90 {
		t.Fatalf("canonical entity was not merged: %#v", result[0])
	}
}

func TestEventsDeduplicateAndSort(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	values := []domain.Event{
		{ID: uuid.New(), OccurredAt: now, Provenance: domain.Provenance{Source: "b", ExternalID: "2"}},
		{ID: uuid.New(), OccurredAt: now.Add(time.Minute), Provenance: domain.Provenance{Source: "a", ExternalID: "1"}},
		{ID: uuid.New(), OccurredAt: now.Add(time.Minute), Provenance: domain.Provenance{Source: "a", ExternalID: "1"}},
	}

	result := Events(values)
	if len(result) != 2 {
		t.Fatalf("got %d events, want 2", len(result))
	}
	if result[0].Provenance.ExternalID != "1" || result[1].Provenance.ExternalID != "2" {
		t.Fatalf("unexpected order: %#v", result)
	}
}
