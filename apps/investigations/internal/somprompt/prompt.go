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

	fmt.Fprintf(&b, "\nThe `investigation` MCP server is configured for project %s. Its Gateway tools reuse the same authenticated project scope; never print or persist credentials.\n\n", c.ProjectID)

	if c.HypothesisID != "" {
		fmt.Fprintf(&b, "Workflow:\n"+
			"1. Read the investigation-wide timeline with `list_investigation_events` for investigation_id %s. Follow pagination until the available context is exhausted.\n"+
			"2. Read the shared graph with `get_investigation_graph` for investigation_id %s without hypothesis_id, then read the active hypothesis projection for investigation_id %s and hypothesis_id %s. Reuse relevant node_id values from either graph; submitting an existing investigation node in the scoped batch adds its hypothesis membership without duplicating it.\n"+
			"3. Before creating edges, read `get_investigation_reference` and use its relation_code endpoint kinds and direction.\n"+
			"4. Investigate additional evidence through the `gateway_*` MCP tools. Start with `gateway_list_sources`, then choose search, aggregation, detail, entity, or endpoint tools supported by the returned capabilities. Use `gateway_resolve_context` to expand selected findings or sessions into canonical events, entities, relations, and per-object resolutions. Use bounded time ranges and the narrowest useful filters. Follow every next_cursor, inspect source_states, source_errors, and resolution status, and never treat partial or truncated output as proof that evidence is absent. If a source is truncated without a cursor, narrow the time range or filters and retry.\n"+
			"5. Submit `add_investigation_agent_results` with investigation_id %s, hypothesis_id %s, and som_issue_ids [\"%s\"]. To import selected Gateway results, declare events by source_code/source_event_id and entities by source_code/source_entity_id, then point nodes at their batch refs using event_ref/entity_ref. Existing attached event_id/entity_id and graph node_id remain valid locators. A node must use exactly one locator. Only explicitly listed nodes and edges gain hypothesis membership; timeline-only events and entities do not.\n"+
			"6. Edges use batch-local node refs in source_ref/target_ref. evidence_event_refs must contain batch-local refs of event nodes from the same batch. Every edge needs a concise evidence-based why; IR stores it as proposed for analyst review.\n"+
			"7. Re-read the hypothesis graph and timeline after the write. Replaying the same batch is safe and should not create duplicate graph facts.\n\n"+
			"Treat the hypothesis statement as an unverified claim: actively seek both supporting and contradicting evidence, and do not turn temporal proximity or correlation into causation. Never invent source identifiers. Copy source_code/source_event_id/source_entity_id or UUIDs exactly from MCP results. If the MCP server is missing or its authentication expired, report that explicitly instead of exposing or retrying with another credential.\n",
			c.InvestigationID, c.InvestigationID, c.InvestigationID, c.HypothesisID, c.InvestigationID, c.HypothesisID, c.SomIssueID)
		return b.String()
	}

	fmt.Fprintf(&b, "Workflow:\n"+
		"1. Read attached context with `list_investigation_events` and `get_investigation_graph` for investigation_id %s. Follow timeline pagination and reuse relevant existing node_id values.\n"+
		"2. Before creating edges, read `get_investigation_reference` and use its relation_code endpoint kinds and direction.\n"+
		"3. Investigate additional evidence through the `gateway_*` MCP tools. Start with `gateway_list_sources`, then choose search, aggregation, detail, entity, or endpoint tools supported by the returned capabilities. Use `gateway_resolve_context` to expand selected findings or sessions into canonical evidence. Use bounded time ranges and the narrowest useful filters. Follow every next_cursor, inspect source_states, source_errors, and resolution status, and never treat partial or truncated output as proof that evidence is absent. If a source is truncated without a cursor, narrow the time range or filters and retry.\n"+
		"4. Submit `add_investigation_agent_results` with investigation_id %s and som_issue_ids [\"%s\"]. To import selected Gateway results, declare events by source_code/source_event_id and entities by source_code/source_entity_id, then point nodes at their batch refs using event_ref/entity_ref. Existing attached event_id/entity_id and graph node_id remain valid locators. A node must use exactly one locator.\n"+
		"5. Edges use batch-local node refs in source_ref/target_ref. evidence_event_refs must contain batch-local refs of event nodes from the same batch. Every edge needs a concise evidence-based why; IR stores it as proposed for analyst review.\n"+
		"6. Re-read graph and timeline after the write. Replaying the same batch is safe and should not create duplicate graph facts.\n\n"+
		"Never invent source identifiers. Copy source_code/source_event_id/source_entity_id or UUIDs exactly from MCP results. If the MCP server is missing or its authentication expired, report that explicitly instead of exposing or retrying with another credential.\n",
		c.InvestigationID, c.InvestigationID, c.SomIssueID)
	return b.String()
}
