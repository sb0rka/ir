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
		"events_found",
		"events_imported",
		"events_total",
		"`filter`/`sort`",
		"Every `nodes[]` entry needs `why`",
		"which slice was imported and the total",
		"imported < found",
		"imported < total",
		"time_range` written in the issue",
		"Wall-clock times in the issue",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"gateway_base_url", "ACCESS_KEY", "http://gateway:8091", "curl", "Bearer eyJ", `Windows accounts need \\`, "suggested time_range:"} {
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

func TestBuildIncludesDisplayTimeZone(t *testing.T) {
	t.Parallel()

	ctx := sampleContext()
	ctx.TimeZone = "Europe/Moscow"
	got := Build("Find events", "time_range 18:53 .. 19:43", ctx)
	if !strings.Contains(got, "display_time_zone: Europe/Moscow") {
		t.Fatalf("missing display_time_zone:\n%s", got)
	}
	if !strings.Contains(got, "Wall-clock times in the issue without an explicit offset/Z are in `display_time_zone`") {
		t.Fatal("missing timezone hard-rule guidance")
	}
}

func TestBuildOmitsDisplayTimeZoneWhenEmpty(t *testing.T) {
	t.Parallel()

	got := Build("Find events", "", sampleContext())
	if strings.Contains(got, "display_time_zone:") {
		t.Fatal("empty time zone must not inject display_time_zone")
	}
}

func TestBuildIncludesResolvedReferences(t *testing.T) {
	t.Parallel()

	ctx := sampleContext()
	ctx.ResolvedEntities = []ResolvedEntity{{
		EntityID: "b71336ed-25f7-42fa-840a-688ceb087c74",
		Type:     "account",
		Value:    `dkrylova\administrator`,
		Sources:  []string{"pt-maxpatrol-siem:account:dkrylova\\administrator"},
	}}
	ctx.UnknownUUIDs = []string{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	got := Build("Find events", "Use entity b71336ed-25f7-42fa-840a-688ceb087c74", ctx)
	for _, want := range []string{
		"Resolved IR references:",
		"entity_id b71336ed-25f7-42fa-840a-688ceb087c74",
		"import_entity_events with entity.entity_id=b71336ed-25f7-42fa-840a-688ceb087c74",
		"unknown UUID aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"time_range` written in the issue",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "suggested time_range:") {
		t.Error("prompt must not inject suggested time_range")
	}
}
