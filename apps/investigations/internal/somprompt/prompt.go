// Package somprompt собирает текст запуска SOM-агента: адреса и заголовки
// должны быть уже развёрнуты, иначе модель оставляет плейсхолдеры в HTTP.
package somprompt

import (
	"fmt"
	"strings"
)

// Context — то, без чего агент не достучится ни до ir-api, ни до Gateway.
type Context struct {
	IRBaseURL             string
	GatewayBaseURL        string
	ProjectID             string
	InvestigationID       string
	HypothesisID          string
	HypothesisStatement   string
	HypothesisDescription string
	SomIssueID            string
}

// Build дописывает к тексту issue адреса, заголовки и правила поиска.
// URL в инструкциях уже развёрнуты: плейсхолдеры вида {investigation_id}
// модель подставляет ненадёжно.
func Build(title, description string, c Context) string {
	ir := strings.TrimRight(c.IRBaseURL, "/")
	gw := strings.TrimRight(c.GatewayBaseURL, "/")
	eventsURL := ir + "/api/v1/investigations/" + c.InvestigationID + "/events"
	graphURL := ir + "/api/v1/investigations/" + c.InvestigationID + "/graph"
	resultsURL := ir + "/api/v1/investigations/" + c.InvestigationID + "/agent-results"
	if c.HypothesisID != "" {
		hypothesisBase := ir + "/api/v1/investigations/" + c.InvestigationID + "/hypotheses/" + c.HypothesisID
		graphURL = hypothesisBase + "/graph"
		resultsURL = hypothesisBase + "/agent-results"
	}
	sourcesURL := gw + "/api/v1/sources"
	searchURL := gw + "/api/v1/events/search"
	irSpec := ir + "/openapi.json"
	gwSpec := gw + "/openapi.yaml"

	var b strings.Builder
	b.WriteString(title)
	if strings.TrimSpace(description) != "" {
		b.WriteString("\n\n")
		b.WriteString(description)
	}

	b.WriteString("\n\n---\nIR context (appended by ir-api):\n")
	writeLine := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", key, value)
		}
	}
	writeLine("ir_api_base_url", ir)
	writeLine("gateway_base_url", gw)
	writeLine("project_id", c.ProjectID)
	writeLine("investigation_id", c.InvestigationID)
	if c.HypothesisID != "" {
		writeLine("hypothesis_id", c.HypothesisID)
		writeLine("hypothesis_statement", c.HypothesisStatement)
		writeLine("hypothesis_description", c.HypothesisDescription)
	}
	writeLine("som_issue_id", c.SomIssueID)

	fmt.Fprintf(&b, "\nMandatory headers on every ir-api and Gateway /api/v1/* request:\n"+
		"  X-Project-ID: %s\n"+
		"  Content-Type: application/json  (POST bodies only)\n"+
		"A missing X-Project-ID returns 400. OpenAPI and health endpoints do not need these headers.\n",
		c.ProjectID)

	fmt.Fprintf(&b, "\nHow to investigate sources and report findings back to IR:\n"+
		"1. Probe reachability first. GET %s with X-Project-ID: %s. Use only the `code` values it returns as `sources` later; do not guess source codes. Then read the investigation timeline and graph with GET %s and GET %s, always sending X-Project-ID. Follow timeline pagination until the available context is exhausted.\n"+
		"2. Search Gateway with POST %s. Start from the issue and attached alert, derive a time window from evidence timestamps, and pivot iteratively on entity type/value pairs copied from results. Keep source_code plus source_event_id/source_entity_id for every record you select.\n"+
		"   Allowed JSON fields only: sources, time_range (from, to), query, entities (each item: type, value), limit (1-100, default 50), cursor. Any other field is rejected with 400.\n"+
		"   query is a plain case-insensitive substring over title, class, and severity — not a query language. Operators and field:value syntax match nothing.\n"+
		"   Entity filters: copy type and value verbatim from a prior response's entities array. Do not invent type names; the OpenAPI list is illustrative and a wrong type matches zero events.\n"+
		"   Time windows: do not assume the data is recent. Take the window from the issue and timeline; when a search is empty, widen rather than narrow. Confirm with an unfiltered search (same sources, no query/entities/time_range) before declaring a source empty.\n"+
		"   Pagination: next_cursor is valid only with otherwise-identical filters. On invalid_cursor, drop cursor and restart from the first page.\n"+
		"3. Persist selected discoveries with POST %s. You may send any number of overlapping batches while investigating. Use som_issue_ids [\"%s\"] in every batch; events and entities may be submitted with empty nodes and edges to enrich the timeline without drawing them. A discovery batch can look like {\"som_issue_ids\":[\"%s\"],\"events\":[{\"ref\":\"selected-event\",\"source_code\":\"...\",\"source_event_id\":\"...\"}],\"entities\":[],\"nodes\":[],\"edges\":[]}. Replaying the same source records and graph facts is idempotent.\n"+
		"4. Before drawing the final graph, read the complete investigation timeline and the assigned graph again. Keep useful benign or false-positive context on the shared timeline, but promote only evidence-backed stages and entities needed for a minimal causal explanation. A new node points to a selected record with event_ref or entity_ref. An existing graph node is reused with node_id; include it in the final batch when it must also be linked to this SOM issue. Every node has a unique batch-local ref.\n"+
		"5. Edges address those node refs through source_ref and target_ref. evidence_event_refs contains batch-local refs of event nodes from the same batch, including event nodes reused through node_id; it never contains event selection refs, source event IDs, or database event IDs. Every edge needs a non-empty evidence-based why. IR assigns agent origin and proposed edge status.\n"+
		"6. Submit the final graph batch, then verify both GET endpoints again.\n",
		sourcesURL, c.ProjectID, eventsURL, graphURL,
		searchURL,
		resultsURL, c.SomIssueID, c.SomIssueID)

	b.WriteString("\nWhen a request is not 2xx, correct it and retry; never end the run on the first error. Handle by error.code:\n" +
		"- bad_request: fix the JSON body (unknown field, missing X-Project-ID, malformed value). Do not retry the same body.\n" +
		"- invalid_cursor: omit cursor and restart pagination with the same filters.\n" +
		"- source_forbidden: re-read GET sources and use only returned codes.\n" +
		"- unsupported_capability: that route has no provider (entity lookup, endpoint search, response actions). Do not retry it.\n" +
		"- 200 with source_errors: partial success; retry only entries with retryable true.\n" +
		"- all_sources_failed or timeout: retry once with a smaller limit or a narrower time window.\n")

	fmt.Fprintf(&b, "If a corrected request still fails, then read GET %s and GET %s. Do not start by reading the specs.\n",
		irSpec, gwSpec)
	if gw != "" {
		b.WriteString("Gateway searches do not write to ir-api; only agent-results persists selected context.\n")
	}
	return b.String()
}
