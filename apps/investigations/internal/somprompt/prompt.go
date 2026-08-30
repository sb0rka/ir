// Package somprompt builds the instructions appended to a SOM issue run.
package somprompt

import (
	"fmt"
	"strings"
)

type Context struct {
	IRBaseURL       string
	ProjectID       string
	InvestigationID string
	SomIssueID      string
}

// Build tells the agent to use only the preconfigured, investigation-scoped
// MCP server. Source credentials and the user's JWT never enter the prompt.
func Build(title, description string, c Context) string {
	var b strings.Builder
	b.WriteString(title)
	if strings.TrimSpace(description) != "" {
		b.WriteString("\n\n")
		b.WriteString(description)
	}

	b.WriteString("\n\n---\nIR context (appended by ir-api):\n")
	fmt.Fprintf(&b, "- investigation_id: %s\n- som_issue_id: %s\n", c.InvestigationID, c.SomIssueID)
	b.WriteString("\nThe `investigation` MCP server is already configured and authorized for exactly this project and investigation. " +
		"Use only its three tools; do not call IR REST, Gateway, Platform API, or Secrets directly.\n\n")
	fmt.Fprintf(&b, "Workflow:\n"+
		"1. Call `list_investigation_events` with investigation_id %s and follow pagination until the attached timeline is exhausted. Record each useful event UUID and entity UUID exactly as returned.\n"+
		"2. Call `get_investigation_graph` with the same investigation_id and reuse relevant existing nodes by node_id.\n"+
		"3. Submit `add_investigation_agent_results` with investigation_id %s and som_issue_ids [\"%s\"]. A new node must use exactly one locator: event_id or entity_id from the attached investigation context, or an existing node_id. Arbitrary nodes and external source references are not supported.\n"+
		"4. Edges use batch-local node refs in source_ref/target_ref. evidence_event_refs must contain batch-local refs of event nodes from the same batch. Every edge needs a concise evidence-based why; IR stores it as proposed for analyst review.\n"+
		"5. Re-read graph and timeline after the write. Replaying the same batch is safe and should not create duplicate graph facts.\n\n"+
		"If a tool rejects an ID, do not guess or substitute a source identifier: re-read the investigation context and use only UUIDs returned by MCP. If a corrected call still fails, report the tool error without attempting another service.\n",
		c.InvestigationID, c.InvestigationID, c.SomIssueID)
	return b.String()
}
