package service

import (
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestRequestFingerprintIncludesAdvancedEventControls(t *testing.T) {
	base := SearchEventsRequest{Sources: []string{"pt-maxpatrol-siem"}, TimeFrom: time.Unix(1, 0), TimeTo: time.Unix(2, 0)}
	changed := base
	changed.Filter = `action = "login"`
	if requestFingerprint(base) == requestFingerprint(changed) {
		t.Fatal("filter must invalidate an existing cursor")
	}
}

func TestSortEventsHonorsRequestedOrder(t *testing.T) {
	events := []domain.Event{
		{OccurredAt: time.Unix(2, 0), Provenance: domain.Provenance{ExternalID: "second"}},
		{OccurredAt: time.Unix(1, 0), Provenance: domain.Provenance{ExternalID: "first"}},
	}
	sortEvents(events, []capability.EventSort{{Field: "time", Direction: "asc"}})
	if events[0].Provenance.ExternalID != "first" {
		t.Fatalf("events were not sorted ascending: %#v", events)
	}
}
