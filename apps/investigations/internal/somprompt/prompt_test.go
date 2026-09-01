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
	if strings.Contains(got, "hypothesis_id:") || strings.Contains(got, "/hypotheses/") {
		t.Error("unscoped prompt contains hypothesis context")
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

func TestBuildScopesGraphAndResultsToHypothesis(t *testing.T) {
	t.Parallel()

	ctx := sampleContext()
	ctx.HypothesisID = "22222222-2222-2222-2222-222222222222"
	ctx.HypothesisStatement = "The account was reused after phishing"
	ctx.HypothesisDescription = "Check authentication and process evidence"
	got := Build("Verify access", "", ctx)
	for _, want := range []string{
		"hypothesis_id: 22222222-2222-2222-2222-222222222222",
		"hypothesis_statement: The account was reused after phishing",
		"hypothesis_description: Check authentication and process evidence",
		"`list_investigation_events`",
		"`get_investigation_graph` for investigation_id 496f2041-7949-4816-8d07-734de89d121f and hypothesis_id 22222222-2222-2222-2222-222222222222",
		"`add_investigation_agent_results` with investigation_id 496f2041-7949-4816-8d07-734de89d121f, hypothesis_id 22222222-2222-2222-2222-222222222222",
		"Only explicitly listed nodes and edges gain hypothesis membership",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "`get_investigation_graph` for investigation_id") && !strings.Contains(got, "hypothesis_id 22222222-2222-2222-2222-222222222222") {
		t.Fatal("hypothesis-scoped graph read must include hypothesis_id")
	}
}
