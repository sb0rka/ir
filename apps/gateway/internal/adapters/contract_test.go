package adapters

import (
	"context"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func TestMockProvidersImplementDeclaredCapabilities(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := NewMockRegistry(value)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(providers.Sources()); got != 5 {
		t.Fatalf("got %d providers, want 5", got)
	}

	for _, source := range providers.Sources() {
		provider, _ := providers.Provider(source.Code)
		for _, declared := range source.Capabilities {
			switch declared {
			case domain.CapabilityEvents:
				_, err = provider.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{Limit: 1})
			case domain.CapabilityEntityLookup:
				_, err = provider.EntityLookup.LookupEntity(context.Background(), capability.LookupEntityRequest{Entity: domain.EntityRef{Type: "ip", Value: "10.125.11.90"}})
			case domain.CapabilityArtifactAnalysis:
				_, err = provider.ArtifactAnalyzer.AnalyzeArtifact(context.Background(), capability.AnalyzeArtifactRequest{Name: "shell.php"})
			case domain.CapabilityEndpoints:
				_, err = provider.Endpoints.SearchEndpoints(context.Background(), capability.SearchEndpointsRequest{Limit: 1})
			case domain.CapabilityResponseCatalog:
				endpoints, endpointErr := provider.Endpoints.SearchEndpoints(context.Background(), capability.SearchEndpointsRequest{Limit: 1})
				if endpointErr != nil || len(endpoints.Items) == 0 {
					t.Fatalf("%s endpoints: %v", source.Code, endpointErr)
				}
				_, err = provider.ResponseCatalog.ListResponseActions(context.Background(), endpoints.Items[0].ExternalID)
			}
			if err != nil {
				t.Fatalf("%s capability %s: %v", source.Code, declared, err)
			}
		}
	}
}
