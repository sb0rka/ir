package somprompt

import (
	"strings"
	"testing"
)

func sampleContext() Context {
	return Context{
		IRBaseURL:       "http://host.docker.internal:8090",
		GatewayBaseURL:  "http://host.docker.internal:8091",
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
		"http://host.docker.internal:8090/api/v1/investigations/496f2041-7949-4816-8d07-734de89d121f/events",
		"http://host.docker.internal:8090/api/v1/investigations/496f2041-7949-4816-8d07-734de89d121f/graph",
		"http://host.docker.internal:8090/api/v1/investigations/496f2041-7949-4816-8d07-734de89d121f/agent-results",
		"http://host.docker.internal:8091/api/v1/sources",
		"http://host.docker.internal:8091/api/v1/events/search",
		"X-Project-ID: abcdef1234",
		"sources, time_range (from, to), query, entities",
		"unsupported_capability",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "- ir_api_base_url: \n") || strings.Contains(got, "- gateway_base_url: \n") {
		t.Error("empty base_url line")
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
	base := "http://host.docker.internal:8090/api/v1/investigations/496f2041-7949-4816-8d07-734de89d121f"
	for _, want := range []string{
		base + "/events",
		base + "/hypotheses/22222222-2222-2222-2222-222222222222/graph",
		base + "/hypotheses/22222222-2222-2222-2222-222222222222/agent-results",
		"hypothesis_id: 22222222-2222-2222-2222-222222222222",
		"hypothesis_statement: The account was reused after phishing",
		"hypothesis_description: Check authentication and process evidence",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, base+"/graph") || strings.Contains(got, base+"/agent-results") {
		t.Fatal("hypothesis-scoped prompt leaked unscoped graph or result URL")
	}
}
