package somprompt

import (
	"fmt"
	"strings"
	"time"
)

type Context struct {
	ProjectID             string
	InvestigationID       string
	HypothesisID          string
	HypothesisStatement   string
	HypothesisDescription string
	SomIssueID            string
	ResolvedEntities      []ResolvedEntity
	ResolvedEvents        []ResolvedEvent
	UnknownUUIDs          []string
	TimelineFrom          *time.Time
	TimelineTo            *time.Time
}

type ResolvedEntity struct {
	EntityID string
	Type     string
	Value    string
	Sources  []string
}

type ResolvedEvent struct {
	EventID       string
	SourceCode    string
	SourceEventID string
	OccurredAt    time.Time
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

	writeResolvedBlock(&b, c)

	b.WriteString("Hard rules:\n")
	b.WriteString("1. Prefer `import_entity_events` when the task is to find events for one entity and add them to the graph. Call it once with the resolved `entity_id` (or type+value) and the suggested time_range. If the issue specifies a predicate or ordering, pass `filter`/`sort` to `import_entity_events`. Use manual `add_investigation_agent_results` only for edges the tool cannot derive.\n")
	b.WriteString("2. IDs: investigation `entity_id`/`event_id`/`node_id` are IR UUIDs. Gateway calls need `type`+`value` or `source_code`+`source_event_id`/`source_entity_id`. Never pass IR UUIDs into gateway entity filters or into `source_*_id` fields.\n")
	b.WriteString("3. Windows account values use a single backslash, e.g. `dkrylova\\administrator`. Doubled backslashes are tolerated by IR/Gateway, but do not invent extra escapes.\n")
	b.WriteString("4. Sources: match capability — accounts/process/auth → SIEM (e.g. pt-maxpatrol-siem); network sessions → NAD. Do not search NAD for Windows accounts.\n")
	b.WriteString("5. Truncated/partial ≠ absent. If `truncated=true` or `events_imported < events_total`, either narrow `time_range`/add `filter`, or state in the report which slice was imported and the total. Follow `next_cursor` when present and inspect `source_states` and `source_errors`. To verify one known `source_event_id`, use `gateway_resolve_context`, not a search filter.\n")
	b.WriteString("6. Write only via `import_entity_events` or `add_investigation_agent_results`. For manual writes: declare Gateway imports in `events`/`entities` with batch-local `ref`, point `nodes` at them with `event_ref`/`entity_ref`, or use attached `event_id`/`entity_id`/`node_id`. Every `nodes[]` entry needs `why`. Edges use batch-local node refs, `why`, and `evidence_event_refs`.\n")
	b.WriteString("7. Never invent source identifiers; copy them exactly from MCP results. If MCP auth fails, report that — do not retry with another credential.\n")
	b.WriteString("8. Final answer must quote write counts verbatim: for `import_entity_events` report `events_total`, `events_found`, and `events_imported` (and `source_errors`/`warnings` when present); for other writes quote `events`/`nodes`/`edges`. Success requires explaining any gap where imported < found or imported < total. Do not claim success after importing a hand-picked subset to satisfy events>0. If no successful write happened, say nothing was written.\n\n")

	if c.HypothesisID != "" {
		fmt.Fprintf(&b, "Workflow: `list_investigation_events` (investigation-wide) → `get_investigation_graph` without hypothesis_id, then with hypothesis_id %s → `get_investigation_reference` → prefer `import_entity_events`, else `gateway_*` as needed → `add_investigation_agent_results` only for leftover edges, with investigation_id %s, hypothesis_id %s, som_issue_ids [\"%s\"] → re-read hypothesis graph. Only listed nodes/edges gain hypothesis membership. Treat the hypothesis as unverified: seek supporting and contradicting evidence; do not turn correlation into causation.\n",
			c.HypothesisID, c.InvestigationID, c.HypothesisID, c.SomIssueID)
		return b.String()
	}

	fmt.Fprintf(&b, "Workflow: `list_investigation_events` + `get_investigation_graph` for investigation_id %s → `get_investigation_reference` → prefer `import_entity_events`, else `gateway_*` (start with `gateway_list_sources`) → `add_investigation_agent_results` only when needed, with investigation_id %s and som_issue_ids [\"%s\"] → re-read graph/timeline. Replay is safe.\n",
		c.InvestigationID, c.InvestigationID, c.SomIssueID)
	return b.String()
}

func writeResolvedBlock(b *strings.Builder, c Context) {
	if len(c.ResolvedEntities) == 0 && len(c.ResolvedEvents) == 0 && len(c.UnknownUUIDs) == 0 &&
		c.TimelineFrom == nil && c.TimelineTo == nil {
		return
	}
	b.WriteString("Resolved IR references:\n")
	for _, entity := range c.ResolvedEntities {
		fmt.Fprintf(b, "- entity_id %s = %s `%s`", entity.EntityID, entity.Type, entity.Value)
		if len(entity.Sources) > 0 {
			fmt.Fprintf(b, " (sources: %s)", strings.Join(entity.Sources, ", "))
		}
		fmt.Fprintf(b, " — use import_entity_events with entity.entity_id=%s or nodes[].entity_id\n", entity.EntityID)
	}
	for _, event := range c.ResolvedEvents {
		fmt.Fprintf(b, "- event_id %s = %s/%s at %s — use nodes[].event_id\n",
			event.EventID, event.SourceCode, event.SourceEventID, event.OccurredAt.UTC().Format(time.RFC3339))
	}
	for _, unknown := range c.UnknownUUIDs {
		fmt.Fprintf(b, "- unknown UUID %s — do not use as a Gateway source id\n", unknown)
	}
	if c.TimelineFrom != nil && c.TimelineTo != nil {
		fmt.Fprintf(b, "- suggested time_range: %s .. %s\n",
			c.TimelineFrom.UTC().Format(time.RFC3339), c.TimelineTo.UTC().Format(time.RFC3339))
	}
	b.WriteString("\n")
}
