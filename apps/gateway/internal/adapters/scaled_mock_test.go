package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func TestScaledMocksSupportRepresentativeFilters(t *testing.T) {
	base, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	value, err := scenario.Expand(base, scenario.GenerateOptions{EventCount: 2_000, EndpointCount: 200, HistoryDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	providers, err := NewMockRegistry(value)
	if err != nil {
		t.Fatal(err)
	}

	siem, _ := providers.Provider("maxpatrol-siem")
	siemPage, err := siem.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Query: "Correlation Alert",
		Limit: 100,
	})
	if err != nil || len(siemPage.Events) == 0 {
		t.Fatalf("SIEM query filter returned events=%d err=%v", len(siemPage.Events), err)
	}
	for _, event := range siemPage.Events {
		if !strings.Contains(strings.ToLower(event.Type+" "+event.Title), "correlation_alert") &&
			!strings.Contains(strings.ToLower(event.Title), "correlation alert") {
			t.Fatalf("unexpected SIEM event: %#v", event)
		}
	}

	entityPage, err := siem.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Entities: []domain.EntityRef{{Type: "host", Value: "ws-000100.corp.example"}},
		Limit:    100,
	})
	if err != nil || len(entityPage.Events) == 0 {
		t.Fatalf("entity filter returned events=%d err=%v", len(entityPage.Events), err)
	}

	nad, _ := providers.Provider("pt-nad")
	nadPage, err := nad.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Query: "Suspicious DNS",
		Limit: 100,
	})
	if err != nil || len(nadPage.Events) == 0 {
		t.Fatalf("NAD query filter returned events=%d err=%v", len(nadPage.Events), err)
	}
	if nadPage.Events[0].Attributes["app_proto"] != "dns" {
		t.Fatalf("unexpected NAD attributes: %#v", nadPage.Events[0].Attributes)
	}
	nadLookup, err := nad.EntityLookup.LookupEntity(context.Background(), capability.LookupEntityRequest{
		Entity: domain.EntityRef{Type: "host", Value: "ws-000100.corp.example"},
	})
	if err != nil || len(nadLookup.Entities) == 0 {
		t.Fatalf("NAD entity lookup returned entities=%d err=%v", len(nadLookup.Entities), err)
	}

	edr, _ := providers.Provider("maxpatrol-edr")
	endpoints, err := edr.Endpoints.SearchEndpoints(context.Background(), capability.SearchEndpointsRequest{
		Query: "ws-000100.corp.example",
		Limit: 10,
	})
	if err != nil || len(endpoints.Items) != 1 || endpoints.Items[0].Hostname != "ws-000100.corp.example" {
		t.Fatalf("endpoint filter returned items=%#v err=%v", endpoints.Items, err)
	}
}
