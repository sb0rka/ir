// Package somprompt builds the instructions appended to a SOM issue run.
package somprompt

import (
	"fmt"
	"strings"
)

type Context struct {
	ProjectID       string
	InvestigationID string
	SomIssueID      string
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
	fmt.Fprintf(&b, "- investigation_id: %s\n- som_issue_id: %s\n", c.InvestigationID, c.SomIssueID)
	fmt.Fprintf(&b, "\nThe `investigation` MCP server is configured for project %s. Its Gateway tools reuse the same authenticated project scope; never print or persist credentials.\n\n", c.ProjectID)
	fmt.Fprintf(&b, "Workflow:\n"+
		"1. Read attached context with `list_investigation_events` and `get_investigation_graph` for investigation_id %s. Follow timeline pagination and reuse relevant existing node_id values.\n"+
		"2. Investigate additional evidence through the `gateway_*` MCP tools. Start with `gateway_list_sources`, then choose search, aggregation, detail, entity, or endpoint tools supported by the returned capabilities. Use bounded time ranges and the narrowest useful filters.\n"+
		"3. Submit `add_investigation_agent_results` with investigation_id %s and som_issue_ids [\"%s\"]. To import selected Gateway results, declare events by source_code/source_event_id and entities by source_code/source_entity_id, then point nodes at their batch refs using event_ref/entity_ref. Existing attached event_id/entity_id and graph node_id remain valid locators. A node must use exactly one locator.\n"+
		"4. Edges use batch-local node refs in source_ref/target_ref. evidence_event_refs must contain batch-local refs of event nodes from the same batch. Every edge needs a concise evidence-based why; IR stores it as proposed for analyst review.\n"+
		"5. Re-read graph and timeline after the write. Replaying the same batch is safe and should not create duplicate graph facts.\n\n"+
		"Never invent source identifiers. Copy source_code/source_event_id/source_entity_id or UUIDs exactly from MCP results. If the MCP server is missing or its authentication expired, report that explicitly instead of exposing or retrying with another credential.\n",
		c.InvestigationID, c.InvestigationID, c.SomIssueID)
	return b.String()
}
