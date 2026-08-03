package adapters

import (
	"context"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func TestProviderNormalizers(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := NewMockRegistry(value)
	if err != nil {
		t.Fatal(err)
	}

	siem, _ := providers.Provider("maxpatrol-siem")
	siemPage, err := siem.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{Limit: 100})
	if err != nil || len(siemPage.Events) == 0 || siemPage.Events[0].Attributes["normalized"] != true {
		t.Fatalf("SIEM normalization failed: events=%d err=%v", len(siemPage.Events), err)
	}

	nad, _ := providers.Provider("pt-nad")
	nadPage, err := nad.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{Limit: 100})
	if err != nil || len(nadPage.Events) == 0 || nadPage.Events[0].Attributes["proto"] != "TCP" {
		t.Fatalf("NAD normalization failed: events=%d err=%v", len(nadPage.Events), err)
	}

	edr, _ := providers.Provider("maxpatrol-edr")
	endpointPage, err := edr.Endpoints.SearchEndpoints(context.Background(), capability.SearchEndpointsRequest{Limit: 100})
	if err != nil || len(endpointPage.Items) == 0 || endpointPage.Items[0].Provenance.Source != "maxpatrol-edr" {
		t.Fatalf("EDR normalization failed: endpoints=%d err=%v", len(endpointPage.Items), err)
	}

	sandbox, _ := providers.Provider("pt-sandbox")
	analysis, err := sandbox.ArtifactAnalyzer.AnalyzeArtifact(context.Background(), capability.AnalyzeArtifactRequest{Name: "shell.php"})
	if err != nil || analysis.Verdict.Value != "malicious" || analysis.Provenance.Source != "pt-sandbox" {
		t.Fatalf("Sandbox normalization failed: analysis=%#v err=%v", analysis, err)
	}

	fusion, _ := providers.Provider("pt-fusion")
	lookup, err := fusion.EntityLookup.LookupEntity(context.Background(), capability.LookupEntityRequest{Entity: domain.EntityRef{Type: "ip", Value: "10.125.11.90"}})
	if err != nil || len(lookup.Verdicts) != 1 || lookup.Verdicts[0].Value != "malicious" || len(lookup.Relations) == 0 {
		t.Fatalf("Fusion normalization failed: result=%#v err=%v", lookup, err)
	}
}
