package somprompt

import (
	"strings"
	"testing"
)

func sampleContext() Context {
	return Context{
		ProjectID:       "abcdef1234",
		InvestigationID: "496f2041-7949-4816-8d07-734de89d121f",
		SomIssueID:      "11111111-1111-1111-1111-111111111111",
	}
}

func TestBuildExpandsURLsAndHeaders(t *testing.T) {
	t.Parallel()

	got := Build("Enrich context", "Look at the attached alert.", sampleContext())

	for _, leftover := range []string{
		"{ir_api_base_url}",
		"{gateway_base_url}",
		"{investigation_id}",
		"{project_id}",
		"{som_issue_id}",
	} {
		if strings.Contains(got, leftover) {
			t.Errorf("unexpanded placeholder %s still present", leftover)
		}
	}
	for _, want := range []string{
		"The `investigation` MCP server is configured",
		"list_investigation_events",
		"get_investigation_graph",
		"add_investigation_agent_results",
		"gateway_list_sources",
		"gateway_*",
		"investigation_id 496f2041-7949-4816-8d07-734de89d121f",
		"event_ref/entity_ref",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"gateway_base_url", "ACCESS_KEY", "http://gateway:8091", "curl", "Bearer eyJ"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("prompt leaks forbidden direct-service instruction %q", forbidden)
		}
	}
}

func TestBuildSkipsEmptyDescription(t *testing.T) {
	t.Parallel()

	got := Build("Enrich context", "  ", sampleContext())
	if !strings.HasPrefix(got, "Enrich context\n\n---\nIR context") {
		t.Fatalf("empty description should not add a dangling separator:\n%s", got[:min(len(got), 120)])
	}
	if strings.Contains(got, "Look at") {
		t.Fatal("blank description leaked into prompt")
	}

	got = Build("Enrich context", "Look at the attached alert.", sampleContext())
	if !strings.Contains(got, "Enrich context\n\nLook at the attached alert.\n\n---\nIR context") {
		t.Fatalf("description should sit between title and IR context:\n%s", got[:min(len(got), 200)])
	}
}
