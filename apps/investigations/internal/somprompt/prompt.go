// Package somprompt builds the instructions appended to a SOM issue run.
package somprompt

import (
	"fmt"
	"strings"
)

type Context struct {
	ProjectID             string
	InvestigationID       string
	HypothesisID          string
	HypothesisStatement   string
	HypothesisDescription string
	SomIssueID            string
}

// Build tells the agent how to use the preconfigured investigation MCP.
func Build(title, description string, c Context) string {
	var b strings.Builder
	b.WriteString(title)
	if strings.TrimSpace(description) != "" {
		b.WriteString("\n\n")
		b.WriteString(description)
	}

	b.WriteString("\n\n---\nIR context (appended by ir-api):\n")
	fmt.Fprintf(&b, "- investigation_id: %s\n", c.InvestigationID)
	if c.HypothesisID != "" {
		fmt.Fprintf(&b, "- hypothesis_id: %s\n", c.HypothesisID)
		if strings.TrimSpace(c.HypothesisStatement) != "" {
			fmt.Fprintf(&b, "- hypothesis_statement: %s\n", c.HypothesisStatement)
		}
		if strings.TrimSpace(c.HypothesisDescription) != "" {
			fmt.Fprintf(&b, "- hypothesis_description: %s\n", c.HypothesisDescription)
		}
	}
	fmt.Fprintf(&b, "- som_issue_id: %s\n", c.SomIssueID)
	fmt.Fprintf(&b, "\nThe `investigation` MCP server is configured for project %s. Gateway tools reuse that authenticated project scope; never print or persist credentials.\n\n", c.ProjectID)

	b.WriteString("Hard rules:\n")
	b.WriteString("1. IDs: investigation `entity_id`/`event_id`/`node_id` are IR UUIDs. Gateway calls need `type`+`value` or `source_code`+`source_event_id`/`source_entity_id`. Never pass IR UUIDs into gateway entity filters.\n")
	b.WriteString("2. Sources: match capability — accounts/process/auth → SIEM (e.g. pt-maxpatrol-siem); network sessions → NAD. Do not search NAD for Windows accounts.\n")
	b.WriteString("3. Truncated/partial ≠ absent. Follow `next_cursor`; if truncated without a cursor, narrow `time_range`/filters and retry. Inspect `source_states` and `source_errors`.\n")
	b.WriteString("4. Write only via `add_investigation_agent_results`: declare Gateway imports in `events`/`entities` with batch-local `ref`, point `nodes` at them with `event_ref`/`entity_ref` (not URNs/objects), or use attached `event_id`/`entity_id`/`node_id`. Edges use batch-local node refs, `why`, and `evidence_event_refs`.\n")
	b.WriteString("5. Never invent source identifiers; copy them exactly from MCP results. If MCP auth fails, report that — do not retry with another credential.\n\n")

	if c.HypothesisID != "" {
		fmt.Fprintf(&b, "Workflow: `list_investigation_events` (investigation-wide) → `get_investigation_graph` without hypothesis_id, then with hypothesis_id %s → `get_investigation_reference` → `gateway_*` as needed → `add_investigation_agent_results` with investigation_id %s, hypothesis_id %s, som_issue_ids [\"%s\"] → re-read hypothesis graph. Only listed nodes/edges gain hypothesis membership. Treat the hypothesis as unverified: seek supporting and contradicting evidence; do not turn correlation into causation.\n",
			c.HypothesisID, c.InvestigationID, c.HypothesisID, c.SomIssueID)
		return b.String()
	}

	fmt.Fprintf(&b, "Workflow: `list_investigation_events` + `get_investigation_graph` for investigation_id %s → `get_investigation_reference` → `gateway_*` (start with `gateway_list_sources`) → `add_investigation_agent_results` with investigation_id %s and som_issue_ids [\"%s\"] → re-read graph/timeline. Replay is safe.\n",
		c.InvestigationID, c.InvestigationID, c.SomIssueID)
	return b.String()
}
