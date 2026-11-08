# Provider mappings

Vendor DTOs and fixed query builders live in `internal/adapters/proxy/<provider>`. Provider contract tests replay sanitized `docs-internal/pt-cases` captures through local HTTP servers and assert method, path, query, body, Cookie handling, timestamps, nested objects, deduplication, and redaction.

## Source catalog

`internal/sources` is the server-side composition boundary. `Default()` registers the PT and Wazuh definitions in one explicit list. A definition owns its stable code, credential selector, ENV loader and credentials decoder. `Catalog.Load` supplies the common source timeout and returns enabled `ConfiguredSource` entries. Their factories capture typed server settings, receive the global TLS policy through `BuildOptions`, and construct existing adapters with the shared safe HTTP client. The HTTP transport uses each entry's `InstanceIDs` for the existing source-instance checks.

To add a source:

1. Implement its typed capability adapter under `internal/adapters/proxy/<provider>` and keep vendor queries, filters and normalization there.
2. Add a definition file in `internal/sources`. Load only declared server ENV variables, validate complete backend settings, and return disabled for an unconfigured backend. Its factory returns `registry.Provider`; names, capabilities and operation implementations remain owned by that provider.
3. Return a typed `capability.Access` from only allowlisted credential fields from the source object passed to the decoder. The catalog selects this object from the request snapshot by the definition's code; the decoder receives neither another source's fields nor the full document. Preserve the difference between a missing field and an explicitly empty value. Never include credential values in errors.
4. Register the definition once in `Default()` and cover its loader, factory and credential contract. Common startup and cookie resolution require no source-name branches. Use the existing Cookie or Basic credential variants; a new authentication method or public capability requires its own explicit boundary change.

`DEMO_PT_SIEM_COOKIE` and `DEMO_PT_NAD_COOKIE` remain internal selectors for the existing `SecretResolver` interface. They do not name independently fetched Secrets or provide an ENV credential fallback. Both user requests and agent requests resolve cookies from their authenticated, project-bound configuration snapshot. The catalog caches no credentials; retry uses the same snapshot, while the next incoming request rechecks current credentials.

The current ENV names and defaults are unchanged. SIEM needs both backend URLs; NAD needs a URL and unique positive store IDs. Partially configured backends fail startup. Cookie assembly retains the existing order and validation: SIEM accepts its complete three-cookie identity set and/or the portal cookie, rejecting a partial identity set; NAD requires both `sessionid` and `csrftoken`. Explicitly empty values remain valid inputs for the vendor to evaluate.

Run Gateway and common-package tests, then `task spec`, `task gateway-spec` and `task check`. Catalog tests include a test-only third definition to exercise extension without adding a production provider. Composition tests cover project JSON, the encrypted agent snapshot, credential removal and internal-header isolation; adapter tests retain the vendor request and normalization coverage.

## MaxPatrol SIEM (`pt-maxpatrol-siem`)

- Optional `created_at_range` applies to incident `createdAt`; `time_range` continues to apply to `detectedAt`. Use `kinds: [siem_incident]` with the creation range. Correlation search has no creation-time filter.
- Incident search/detail uses the Incident Read Model backend and follows only confirmed offset pagination for child events, accounts, files, links, and asset groups.
- Correlations are firing events from `POST /api/events/v3/events` (PDQL filter with `correlation_name != null`). Exact resolution retrieves the UUID and then each listed subevent.
- Event search accepts an allowlisted predicate, columns, ordered sort rules, and a selected group path. The adapter composes the PDQL pipeline and rejects arbitrary pipeline fragments, comments, unknown fields, and misaligned group values. The predicate allows comparisons, `contains`, null checks and `and`/`or`/`not`. `in`, `==`, functions and pipeline stages are outside the allowlist. Common host and IP conditions stay outside the PDQL text.
- Event and correlation lists page by the vendor HTTP `offset`/`limit` (no PDQL `limit()`, so `totalCount` stays the global match count). The first page asks for `noCount=false` and exposes the vendor total; continuation pages use `noCount=true` and stop on a short page or when the known total is reached. The `cursor` carries only the next offset.
- Event aggregation calls `POST /api/events/v3/events/aggregation` through a fixed PDQL template. Public `count` sorting maps to the private `Cnt` alias; group values are bounded strings or null and counts must be non-negative integers. PT NAD does not implement this narrower operation.
- Incident resolution emits child correlation firings as first-class findings. Correlation and subevents also become granular events; subevents carry bounded `parent_source_event_id` and `relation_type=subevent_of` metadata for graph decomposition.
- `/api/siem/v2/rules/correlation` is intentionally excluded because no reviewed response capture defines rule objects.
- Account userinfo and health probes use the same per-call project cookie without storing it in the client.

SIEM object identity is `pt-maxpatrol-siem + empty source_instance + kind + UUID`.

## PT NAD (`pt-nad`)

- Event search has no native filter language. Common host and IP match normalized entity mentions inside the available window. Native columns, sort and event grouping are not implemented.
- Session and attack search fan out only across process-configured store IDs using fixed BQL templates. Optional `filter` is compiled from an allowlisted predicate before `ORDER BY` and `LIMIT`, matching the captured NAD UI semantics: attack search applies its predicate inside the parent flow, including nested alert/SMB predicates.
- Session resolution calls `/api/v2/flow/{id}` with the original store and time window. When the flow detail reaches PT NAD's 100-item nested HTTP limit, the adapter follows with fixed exact-session BQL pages by increasing `tx_id` until an empty proof page; a failed or bounded-out continuation keeps the root session and marks its resolution `partial`.
- Attack resolution uses an exact alert lookup to determine the parent, then calls `GET /api/v2/flow/{flow_id}/alert/{alert_id}` and verifies both IDs. Dedicated detail supplies malware family, signature metadata and a payload evidence reference. The parent session is first-class; failed enrichment preserves the root and marks it partial.
- Session criticality uses the reviewed normalized scale. Conflicting alert priority semantics yield `severity=unknown` while preserving `raw_priority`.
- Flow metadata includes `mail_hints` (from/to/subject/date). `network.mail` events mention email entities and carry `parent_session_id`; credentials accept `login` with fallback to `user`. Passwords and NTLM material stay excluded. A file observed in the same session is not automatically attached to a particular email.
- Ordinary search/context responses contain metadata and evidence references. Full payload and selected-file content is available only through the explicit [evidence export API](evidence.md). File export follows the captured `sources/getfile -> tasks/{id} -> download` ZIP chain. File download support is retained; pilot checks must use supplied dumps or synthetic content instead of downloading malware or suspicious attachments. PCAP references are preserved, but NAD session-PCAP export returns `unsupported_capability`; course dumps are supplied and analyzed separately.

NAD object identity is `pt-nad + store_id + kind + vendor_id`; identical vendor IDs in different stores do not collapse.

## Unregistered canonical capabilities

Artifact analysis, endpoint inventory, response catalog, and account contracts remain in the canonical API, but no mock, dummy, or `pt-sandbox` provider backs them. A future source must add a reviewed real client before it can be allowlisted.

## Wazuh (`wazuh`)

The adapter implements event search, exact event/entity context, event aggregation and an index-scoped search probe. Wazuh alerts remain events; frequency rules do not create MaxPatrol correlation findings. Findings, sessions, Manager API operations and raw vendor proxying are not advertised.

Server settings are `SOURCE_WAZUH_BASE_URL`, `SOURCE_WAZUH_INDEX_PATTERN` (default `wazuh-alerts-*`), `SOURCE_WAZUH_TIMEOUT_SEC` (common source timeout by default), `SOURCE_WAZUH_TLS_CA_FILE`, and `SOURCE_WAZUH_SKIP_TLS_VERIFY` (default false). The latter opts out for Wazuh only; the existing global skip flag remains effective. URL absence disables the backend; enabled settings are validated at startup. The index pattern is a single index-name glob with a literal prefix; custom alert prefixes are supported and every document reference is checked against that server pattern. Project JSON supplies `name`, `username` and `password`; URLs, indexes, TLS settings and unrelated fields in that JSON cannot override server policy.

Occurrence time and the `time` query alias use `timestamp`; exact detail can fall back to `@timestamp` when necessary. Queries use bounded predicates (`=`, `!=`, ranges, `and/or/not`, `in`, `contains`, null checks) compiled to fixed Query DSL, never arbitrary DSL or `query_string`. DQL accepts `field:value`, quoted strings, numeric and absolute ranges, `AND`/`OR`/`NOT`, `field:(a OR b)` and existence `field:*`. An unquoted `*` is a wildcard only on keyword fields, at most 256 characters and four stars; a quoted star is literal. `NOT field:value` also matches documents where the field is absent. Limits are 4096 bytes, nesting 16 and 128 clauses. Field-name wildcards, unfielded text, relative time, fuzzy syntax and raw DSL are rejected. The private field vocabulary in `fields.go` controls filtering, columns, sorting and grouping. Useful examples are `rule.level >= 5`, `rule.id in ("5710", "5712")`, `agent.name = "workstation"`, and `data.srcip = "192.0.2.1"`. PT fields such as `uuid` and `correlation_type` are not aliases. Date literals must be absolute (RFC3339, Wazuh timestamps, calendar dates or epoch milliseconds); relative date math would change the query during PIT pagination and is rejected.

Event identity is the full `<_index>/<_id>` pair. `_mget` uses validated indexes and an allowlisted source projection, preserves found documents when another document is missing, and verifies response identities. Entity IDs use `type:canonical_value` and require a confirming alert search. Rule, agent, Windows, network/account and FIM metadata become bounded canonical evidence; raw logs, arbitrary vendor JSON and credentials are excluded. CVSS numbers tolerate the number/string forms emitted by Wazuh decoders.

Search pages use a PIT, `search_after`, and stable sort keys: requested fields, then `timestamp desc`, `_index asc`, and `_id asc`. On OpenSearch 2.19 this compatible `_id` sort uses fielddata and can consume memory proportional to the participating shards. `SOURCE_WAZUH_DOCUMENT_ID_FIELD` can name a keyword copy of `_id` with doc_values and identical per-document values to avoid that cost. The operator owns its mapping/backfill; Gateway never alters index settings, ingest pipelines or documents. The current installation uses the compatible `_id` default.

Aggregation scans composite pages using the server's `after_key` and includes missing fields as null buckets. A bounded heap retains top-N while preserving requested numeric/count ordering; an incomplete scan returns a source error instead of claiming a global top-N. Groups remain source-local.

For REST/MCP, select `sources: ["wazuh"]` to use its native controls. MCP import takes totals from the search response instead of a PT-specific grouping field. Automatic MCP source selection and a UI/MCP field catalog are deferred.

An optional read-only verification uses `WAZUH_LIVE_BASE_URL`, `WAZUH_LIVE_USERNAME`, `WAZUH_LIVE_PASSWORD`, and `WAZUH_LIVE_SKIP_TLS_VERIFY=true` with `go test ./apps/gateway/internal/adapters/proxy/wazuh -run TestLiveIndexerReadOnly -v`. It reads alerts, resolves context and groups events; all PITs it opens are released.
