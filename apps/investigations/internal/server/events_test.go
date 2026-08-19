package server

import (
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

func TestConvertEventSummaryIncludesEntities(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	item := model.EventSummary{
		ID:             "496f2041-7949-4816-8d07-734de89d121f",
		SourceCode:     "maxpatrol-siem",
		SourceEventID:  "evt-1",
		Title:          "Process started",
		EventType:      "process_start",
		OccurredAt:     occurred,
		IngestedAt:     occurred,
		AttachedAt:     occurred,
		AttachedBy:     "agent",
		NormalizedData: []byte(`{"severity":"high"}`),
		Entities: []model.EventEntity{
			{EntityID: "11111111-1111-1111-1111-111111111111", RelationCode: "actor"},
			{EntityID: "22222222-2222-2222-2222-222222222222", RelationCode: "host"},
		},
	}

	got, err := convertEventSummary(item)
	if err != nil {
		t.Fatal(err)
	}
	if got.Entities == nil {
		t.Fatal("entities omitted from event summary")
	}
	if len(*got.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(*got.Entities))
	}
	first := (*got.Entities)[0]
	if first.EntityId.String() != "11111111-1111-1111-1111-111111111111" || first.RelationCode != "actor" {
		t.Fatalf("first entity = %+v", first)
	}
}

func TestConvertEventSummaryEmptyEntities(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	got, err := convertEventSummary(model.EventSummary{
		ID:             "496f2041-7949-4816-8d07-734de89d121f",
		SourceCode:     "maxpatrol-siem",
		SourceEventID:  "evt-1",
		Title:          "Process started",
		EventType:      "process_start",
		OccurredAt:     occurred,
		IngestedAt:     occurred,
		AttachedAt:     occurred,
		AttachedBy:     "analyst",
		NormalizedData: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Entities == nil || len(*got.Entities) != 0 {
		t.Fatalf("entities = %#v, want empty slice", got.Entities)
	}
}

func TestConvertEventSummaryRejectsMalformedEntityID(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	_, err := convertEventSummary(model.EventSummary{
		ID:             "496f2041-7949-4816-8d07-734de89d121f",
		SourceCode:     "maxpatrol-siem",
		SourceEventID:  "evt-1",
		Title:          "Process started",
		EventType:      "process_start",
		OccurredAt:     occurred,
		IngestedAt:     occurred,
		AttachedAt:     occurred,
		AttachedBy:     "analyst",
		NormalizedData: []byte(`{}`),
		Entities:       []model.EventEntity{{EntityID: "not-a-uuid", RelationCode: "actor"}},
	})
	if err == nil {
		t.Fatal("malformed entity id accepted")
	}
}
