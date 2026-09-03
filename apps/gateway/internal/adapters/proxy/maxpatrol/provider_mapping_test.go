package maxpatrol

import (
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func TestFindingFromIncidentMapsType(t *testing.T) {
	t.Parallel()
	incident := Incident{
		ID:         "1e3e97cb-5540-0001-0000-0000000002b1",
		Key:        "INC-238",
		Name:       "Possible_Network_Local_Tunnel",
		Severity:   "medium",
		DetectedAt: time.Date(2025, 10, 23, 15, 53, 40, 0, time.UTC),
		Type:       "MalwareInfection",
		State:      "New",
	}
	finding := findingFromIncident(incident, domain.TimeRange{
		From: time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
	}, time.Now().UTC(), nil)
	if finding.Incident == nil {
		t.Fatal("expected incident details")
	}
	if finding.Incident.Type != "MalwareInfection" {
		t.Fatalf("incident type = %q, want MalwareInfection", finding.Incident.Type)
	}
}
