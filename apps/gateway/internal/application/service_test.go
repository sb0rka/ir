package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func TestSearchEventsFanOutAndCursorAfterFiltering(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := adapters.NewMockRegistry(value)
	if err != nil {
		t.Fatal(err)
	}
	service := New(providers, time.Second, time.Second)

	first, err := service.SearchEvents(context.Background(), SearchEventsRequest{
		Sources: []string{"maxpatrol-siem", "pt-nad"}, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 || first.NextCursor == "" {
		t.Fatalf("unexpected first page: events=%d cursor=%q", len(first.Events), first.NextCursor)
	}
	if first.Events[0].OccurredAt.Before(first.Events[1].OccurredAt) {
		t.Fatal("events are not sorted newest first")
	}

	second, err := service.SearchEvents(context.Background(), SearchEventsRequest{
		Sources: []string{"maxpatrol-siem", "pt-nad"}, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events) == 0 {
		t.Fatal("second page is empty")
	}
	seen := map[string]bool{}
	for _, event := range first.Events {
		seen[event.Provenance.Source+":"+event.Provenance.ExternalID] = true
	}
	for _, event := range second.Events {
		if seen[event.Provenance.Source+":"+event.Provenance.ExternalID] {
			t.Fatalf("event repeated across pages: %s/%s", event.Provenance.Source, event.Provenance.ExternalID)
		}
	}
}

func TestSearchEventsRejectsCursorForDifferentFilter(t *testing.T) {
	value, _ := scenario.Load(fixtures.Investigation)
	providers, _ := adapters.NewMockRegistry(value)
	service := New(providers, time.Second, time.Second)
	first, err := service.SearchEvents(context.Background(), SearchEventsRequest{Sources: []string{"maxpatrol-siem"}, Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %v, cursor=%q", err, first.NextCursor)
	}
	_, err = service.SearchEvents(context.Background(), SearchEventsRequest{Sources: []string{"maxpatrol-siem"}, Query: "different", Limit: 1, Cursor: first.NextCursor})
	var requestErr *domain.RequestError
	if !errors.As(err, &requestErr) || requestErr.Code != "invalid_cursor" {
		t.Fatalf("got %v, want invalid_cursor", err)
	}
}

func TestSearchEventsReturnsPartialFailure(t *testing.T) {
	goodEvent := domain.Event{ID: domain.StableID("event", "good", "1"), OccurredAt: time.Now(), Provenance: domain.Provenance{Source: "good", ExternalID: "1"}}
	providers, err := registry.New(
		registry.Provider{Source: eventSource("good"), Events: fakeEvents{page: capability.EventPage{Events: []domain.Event{goodEvent}, Continuations: []string{"1"}}}},
		registry.Provider{Source: eventSource("bad"), Events: fakeEvents{err: errors.New("down")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(providers, time.Second, time.Second).SearchEvents(context.Background(), SearchEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || len(result.SourceErrors) != 1 || result.SourceErrors[0].Source != "bad" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSearchEventsReturnsBadGatewayConditionWhenAllFail(t *testing.T) {
	providers, _ := registry.New(
		registry.Provider{Source: eventSource("one"), Events: fakeEvents{err: errors.New("down")}},
		registry.Provider{Source: eventSource("two"), Events: fakeEvents{err: errors.New("down")}},
	)
	_, err := New(providers, time.Second, time.Second).SearchEvents(context.Background(), SearchEventsRequest{})
	if !errors.Is(err, domain.ErrAllSourcesFailed) {
		t.Fatalf("got %v, want ErrAllSourcesFailed", err)
	}
}

func TestSearchEventsEnforcesOverallTimeout(t *testing.T) {
	providers, _ := registry.New(
		registry.Provider{Source: eventSource("slow"), Events: fakeEvents{delay: 250 * time.Millisecond}},
	)
	started := time.Now()
	_, err := New(providers, 30*time.Millisecond, 10*time.Millisecond).SearchEvents(context.Background(), SearchEventsRequest{})
	if !errors.Is(err, domain.ErrAllSourcesFailed) {
		t.Fatalf("got %v, want ErrAllSourcesFailed", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("overall timeout was not enforced: %s", elapsed)
	}
}

type fakeEvents struct {
	page  capability.EventPage
	err   error
	delay time.Duration
}

func (fake fakeEvents) SearchEvents(context.Context, capability.SearchEventsRequest) (capability.EventPage, error) {
	if fake.delay > 0 {
		time.Sleep(fake.delay)
	}
	return fake.page, fake.err
}

func eventSource(code string) domain.Source {
	return domain.Source{Code: code, Name: code, Kind: "siem", Mode: "mock", Status: "available", Capabilities: []domain.Capability{domain.CapabilityEvents}}
}
