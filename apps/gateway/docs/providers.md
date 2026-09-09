# Provider mappings

Vendor DTOs and fixed query builders live in `internal/adapters/proxy/<provider>`. Provider contract tests replay sanitized `docs-internal/pt-cases` captures through local HTTP servers and assert method, path, query, body, Cookie handling, timestamps, nested objects, deduplication, and redaction.

## MaxPatrol SIEM (`pt-maxpatrol-siem`)

- Optional `created_at_range` applies to incident `createdAt`; `time_range` continues to apply to `detectedAt`. Use `kinds: [siem_incident]` with the creation range. Correlation search has no creation-time filter.
- Incident search/detail uses the Incident Read Model backend and follows only confirmed offset pagination for child events, accounts, files, links, and asset groups.
- Correlations are firing events from `POST /api/events/v3/events` (PDQL filter with `correlation_name != null`). Exact resolution retrieves the UUID and then each listed subevent.
- Event search accepts an allowlisted predicate, columns, ordered sort rules, and a selected group path. The adapter composes the PDQL pipeline and rejects arbitrary pipeline fragments, comments, unknown fields, and misaligned group values.
- Event and correlation lists page by the vendor HTTP `offset`/`limit` (no PDQL `limit()`, so `totalCount` stays the global match count). The first page asks for `noCount=false` and exposes the vendor total; continuation pages use `noCount=true` and stop on a short page or when the known total is reached. The `cursor` carries only the next offset.
- Event aggregation calls `POST /api/events/v3/events/aggregation` through a fixed PDQL template. Public `count` sorting maps to the private `Cnt` alias; group values are bounded strings or null and counts must be non-negative integers. PT NAD does not implement this narrower operation.
- Incident resolution emits child correlation firings as first-class findings. Correlation and subevents also become granular events; subevents carry bounded `parent_source_event_id` and `relation_type=subevent_of` metadata for graph decomposition.
- `/api/siem/v2/rules/correlation` is intentionally excluded because no reviewed response capture defines rule objects.
- Account userinfo and health probes use the same per-call project cookie without storing it in the client.

SIEM object identity is `pt-maxpatrol-siem + empty source_instance + kind + UUID`.

## PT NAD (`pt-nad`)

- Session and attack search fan out only across process-configured store IDs using fixed BQL templates. Optional `filter` is compiled from an allowlisted predicate before `ORDER BY` and `LIMIT`, matching the captured NAD UI semantics: attack search applies its predicate inside the parent flow, including nested alert/SMB predicates.
- Session resolution calls `/api/v2/flow/{id}` with the original store and time window. When the flow detail reaches PT NAD's 100-item nested HTTP limit, the adapter follows with fixed exact-session BQL pages by increasing `tx_id` until an empty proof page; a failed or bounded-out continuation keeps the root session and marks its resolution `partial`.
- Attack resolution uses an exact alert lookup to determine the parent, then calls `GET /api/v2/flow/{flow_id}/alert/{alert_id}` and verifies both IDs. Dedicated detail supplies malware family, signature metadata and a payload evidence reference. The parent session is first-class; failed enrichment preserves the root and marks it partial.
- Session criticality uses the reviewed normalized scale. Conflicting alert priority semantics yield `severity=unknown` while preserving `raw_priority`.
- Flow metadata includes `mail_hints` (from/to/subject/date). `network.mail` events mention email entities and carry `parent_session_id`; credentials accept `login` with fallback to `user`. Passwords and NTLM material stay excluded. A file observed in the same session is not automatically attached to a particular email.
- Ordinary search/context responses contain metadata and evidence references. Full alert payload is available through the explicit [evidence export API](evidence.md). File extraction is disabled for the pilot: file names, hashes and relations remain metadata only, and no executable or attachment is downloaded. File requests return `unsupported_capability` before any NAD request. PCAP references are provenance only; use the dumps supplied with the case.

NAD object identity is `pt-nad + store_id + kind + vendor_id`; identical vendor IDs in different stores do not collapse.

## Unregistered canonical capabilities

Artifact analysis, endpoint inventory, response catalog, and account contracts remain in the canonical API, but no mock, dummy, or `pt-sandbox` provider backs them. A future source must add a reviewed real client before it can be allowlisted.
