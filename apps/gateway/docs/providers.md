# Provider mappings

Vendor DTOs and fixed query builders live in `internal/adapters/proxy/<provider>`. Provider contract tests replay sanitized `docs-internal/pt-cases` captures through local HTTP servers and assert method, path, query, body, Cookie handling, timestamps, nested objects, deduplication, and redaction.

## MaxPatrol SIEM (`pt-maxpatrol-siem`)

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

- Session and attack search fan out only across process-configured store IDs using fixed BQL templates.
- Session resolution calls `/api/v2/flow/{id}` with the original store and time window. When the flow detail reaches PT NAD's 100-item nested HTTP limit, the adapter follows with fixed exact-session BQL pages by increasing `tx_id` until an empty proof page; a failed or bounded-out continuation keeps the root session and marks its resolution `partial`.
- Attack resolution uses an exact escaped alert ID, resolves its parent session, and obtains the complete safe flow detail. The related session is first-class.
- Session criticality uses the reviewed normalized scale. Conflicting alert priority semantics yield `severity=unknown` while preserving `raw_priority`.
- Flow metadata is reduced to safe protocol, endpoint, traffic, state, file, and authentication hints. Payloads, cookies, credentials, NTLM material, PCAP, and file contents are discarded.

NAD object identity is `pt-nad + store_id + kind + vendor_id`; identical vendor IDs in different stores do not collapse.

## Wazuh (`wazuh`)

- Reads only `wazuh-alerts-*` (configurable pattern) from the Wazuh Indexer /
  OpenSearch HTTP API. Archives, inventory, and vulnerability-state indices are
  out of scope.
- Authenticates with process-level Basic Auth (`SOURCE_WAZUH_USERNAME` /
  `SOURCE_WAZUH_PASSWORD`). Credentials never enter project Secrets, public
  requests, or logs. `CredentialMode=process` skips the Secret resolver.
- TLS uses a Wazuh-specific `SOURCE_WAZUH_INSECURE_SKIP_VERIFY` flag; it does
  not reuse `GATEWAY_SKIP_TLS_VERIFY`.
- Event search builds a fixed Query DSL: required `@timestamp` range, optional
  entity `should` clauses, optional PDQL-compatible predicate (`filter`),
  `group_by`/`group_values`, `_source` includes, and `search_after` pagination
  with sort `[..., @timestamp, id, _index, _id]`.
- The filter allowlist is Wazuh-native (`rule.groups`, `rule.level`,
  `agent.name`, `data.srcip`, `syscheck.*`, …) plus canonical `time`, `uuid`,
  `correlation_type`, and `correlation_name`. MaxPatrol field aliases are not
  accepted; unknown fields fail only the Wazuh source.
- Event attributes use the same native keys as the filter allowlist so the
  dashboard card and PDQL filters share one vocabulary.
- Frequency rules (`rule.frequency`) emit `correlation_name` and
  `correlation_type=wazuh_frequency` (other rules use `wazuh_rule`). Each alert
  remains a standalone event; there is no `correlation_event_id` collapse.
- Aggregation supports terms/composite groups plus a synthetic
  `correlation_type` filters aggregation used by `import_entity_events`.
- Identity is `wazuh + <_index>/<_id>`. Resolve rejects indexes outside the
  configured alerts pattern. `full_log` and `previous_output` are never returned.

Wazuh object identity for events is `wazuh + empty source_instance + source_event_id`.

## Unregistered canonical capabilities

Artifact analysis, endpoint inventory, response catalog, and account contracts remain in the canonical API, but no mock, dummy, or `pt-sandbox` provider backs them. A future source must add a reviewed real client before it can be allowlisted.
