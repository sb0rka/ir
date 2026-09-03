package somprompt

import (
	"strings"
	"testing"
	"time"
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
		"import_entity_events",
		"get_investigation_reference",
		"gateway_list_sources",
		"gateway_resolve_context",
		"gateway_*",
		"next_cursor",
		"source_states",
		"source_errors",
		"investigation_id 496f2041-7949-4816-8d07-734de89d121f",
		"event_ref`/`entity_ref",
		`dkrylova\administrator`,
		"nothing was written",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"gateway_base_url", "ACCESS_KEY", "http://gateway:8091", "curl", "Bearer eyJ", `Windows accounts need \\`} {
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
		"`get_investigation_graph` without hypothesis_id",
		"with hypothesis_id 22222222-2222-2222-2222-222222222222",
		"`add_investigation_agent_results` only for leftover edges, with investigation_id 496f2041-7949-4816-8d07-734de89d121f, hypothesis_id 22222222-2222-2222-2222-222222222222",
		"Only listed nodes/edges gain hypothesis membership",
		"supporting and contradicting evidence",
		"do not turn correlation into causation",
		"import_entity_events",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBuildIncludesResolvedReferences(t *testing.T) {
	t.Parallel()

	from := time.Date(2025, 10, 22, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 10, 24, 0, 0, 0, 0, time.UTC)
	ctx := sampleContext()
	ctx.ResolvedEntities = []ResolvedEntity{{
		EntityID: "b71336ed-25f7-42fa-840a-688ceb087c74",
		Type:     "account",
		Value:    `dkrylova\administrator`,
		Sources:  []string{"pt-maxpatrol-siem:account:dkrylova\\administrator"},
	}}
	ctx.UnknownUUIDs = []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	ctx.TimelineFrom = &from
	ctx.TimelineTo = &to
	got := Build("Find events", "Use entity b71336ed-25f7-42fa-840a-688ceb087c74", ctx)
	for _, want := range []string{
		"Resolved IR references:",
		"entity_id b71336ed-25f7-42fa-840a-688ceb087c74",
		"import_entity_events with entity.entity_id=b71336ed-25f7-42fa-840a-688ceb087c74",
		"unknown UUID aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"suggested time_range: 2025-10-22T00:00:00Z .. 2025-10-24T00:00:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}
