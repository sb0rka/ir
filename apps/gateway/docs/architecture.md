# Gateway architecture

## Boundary

```text
HTTP client
  -> transport/http (auth, project grants, OpenAPI validation)
  -> service (current project credentials, retry, fan-out, completeness)
  -> registry (provider selection by capability)
  -> provider adapter (typed vendor DTOs and fixed queries)
  -> normalization (canonical values, merge, sort)
```

Gateway owns no database, investigation, vendor URL, or credential supplied by a public request. Process configuration fixes URLs, TLS policy, and NAD store IDs. IR stores the project's ordinary JSON secret `IR/SOURCE_CONFIGURATION` as a JSON source array; Gateway selects typed credentials only from the matching source name. The domain and service packages do not depend on HTTP, so a future MCP transport must use the same service boundary.

Providers register only implemented capabilities. The composition root creates one `internal/sources` catalog and passes it to process configuration loading, registry construction and project credential resolution. The catalog's static definitions for `pt-maxpatrol-siem`, `pt-nad` and `wazuh` own their ENV loaders, validation, adapter factories and credential decoders. A configured source captures typed server settings and exposes only its code, permitted instance IDs and factory; no vendor credentials are retained there. Names and capabilities come from the constructed provider, rather than a second metadata list.

Empty backend settings produce an empty registry and partial settings fail startup. Catalog construction rejects duplicate codes and credential selectors; registry construction also verifies that a factory returned the expected provider code and selector. Project availability follows the current usable credentials. No manual project/source mapping is used. Mock and Sandbox providers are not registered and there is no generic proxy fallback. See [source catalog](providers.md#source-catalog) for the extension workflow.

Search calls fan out concurrently to the selected allowed providers. Each response carries `complete`, `truncated`, or `failed` source state. Event pagination merges ordered provider streams without dropping partly consumed pages. An encrypted, signed project-bound cursor retains each page start, original size, consumed count and source totals; it contains no event data or credentials. Wazuh supplies a PIT-backed replay position even for the first page. SIEM retains native offset continuation, while NAD can replay only its bounded existing window and remains truncated when that window is consumed. Other operation cursors are unchanged.

Event aggregation uses the same project-scoped fan-out and retry boundary, but returns source-local groups instead of canonical event records. Counts from different sources are never merged. A provider may support event search without implementing the narrower aggregation interface; mixed requests report that source as unsupported while retaining successful groups from other sources.

## Event query

A search or aggregate request carries one `filter` and one `filter_language`. Common conditions combine with that single native predicate. A PT predicate is not compiled into Wazuh, or the reverse.

Omitted or empty `sources` selects every project-available event source. An explicit unavailable source is `source_forbidden`. `filter_language` keeps only sources that implement it, including when `filter` is empty; no compatible source is `unsupported_filter_language`. A language or a non-empty `common_filter` validates fields, operators and controls on every remaining source before any vendor call. Rejection cancels the request; unsupported controls are not removed to widen the result. With neither a language nor a filled `common_filter`, every selected source stays in the fan-out and adapter failures stay partial `source_errors`.

The predicate is time AND any listed host AND any listed IP AND any entity ref AND the native filter AND the group drill-down. A missing clause does not constrain. `entities` is an OR of canonical `{type, value}` pairs, not IR entity UUIDs; host-AND-IP belongs in `common_filter`. A common host or IP is any participating value, not a `src`, `dst` or agent role.

Paths live in the embedded `internal/fieldmap/mappings.json`. Startup checks structure, duplicates, paths and the native catalogs of enabled adapters. `mapping_version` is the SHA-256 of that file and is part of the event-cursor binding. Hosts are trimmed, lower-cased and stripped of one trailing dot; a short name is not treated as an FQDN. IPs are canonical; CIDR and wildcards are rejected. Limits are 100 values and 253 bytes. Wazuh common-host terms are case-insensitive; native Wazuh exact and wildcard terms keep case. Accounts, severity, free text and rule identifiers are not common filters. Display severity is normalization only.

`time_range` is absolute RFC3339 and is converted to UTC. Wazuh bounds are inclusive. The API does not promise a half-open `[from, to)` for every product.

NAD event search has no filter language. Common host and IP match normalized entity mentions inside the vendor window. Native columns, sort and grouping are unsupported, so a strict request that still includes NAD rejects those controls.

Page `entities` and `relations` describe that page. Identity is `source_code` + `source_event_id`. `total` sums known totals of non-failed sources and is omitted when any of them has no total. It is not the page length. Pagination ends when `next_cursor` is absent. `complete` is not the last page; MaxPatrol may mark a full non-final page `truncated`. An empty page with `source_errors` is not an empty match set. Partial success is HTTP 200. Failure of every selected source is HTTP 502 `all_sources_failed`, except legacy input errors returned as HTTP 400.

Continuation may change only `limit` and must use the cursor from that response. There is no cross-product snapshot. After a source failure, later pages are not a globally ordered snapshot. NAD identity filters read at most 5000 records per configured store, separately for sessions and attacks, and that window has no total.

Aggregate `limit` (1–1000, default 100) caps groups per source, so the merged array can be larger. JSON `null` in group values is a missing field; the string `"null"` is literal. A finished Wazuh top-N may be `truncated` when further groups exist; counts of the returned groups are exact. An interrupted scan is a source error and contributes no groups. Drill-down is a new search on that source alone, with aligned `group_by` and `group_values` and its own snapshot. Grouping alerts does not create a finding.

Every request checks both grants, the live session and current API RBAC. One request-bound JSON snapshot supplies all credential selections; plaintext is not cached between requests. Missing or invalid credential fields exclude that source before any vendor call, including when its previous online status is cached. Backend statuses alone have a short cache. Retryable network/timeout, `401/403`, or `5xx` failures may repeat an operation once with the same request snapshot. Redirects are rejected before credentials can move to another request.

## Object identity and context

Finding and session identity is `{source_code, source_instance, record_type, external_id}`. The required time range is replay provenance and is not part of identity. SIEM uses an empty source instance; NAD uses a configured store ID.

`Finding` and `Session` are first-class coarse objects. `Event`, `Entity`, and entity `Relation` remain granular evidence. Resolve retains a found root even when child context fails and marks that object `partial`; a missing root is not synthesized. Incident resolution includes correlation findings. Attack resolution includes its parent network session. Ordinary object responses contain metadata and evidence references. Explicit evidence exports stream payload or file bytes; cookies, password/NTLM material and full vendor JSON stay behind the adapter boundary. The course supplies PCAP dumps separately; NAD session-PCAP export remains unsupported.

Canonical normalization covers IP, MAC, host, account, and hashes. Event entity mentions retain roles such as `src`, `dst`, `attacker`, and `victim`; flow direction never substitutes for attacker semantics.

## Temporary evidence exports

A bounded process-local registry owns up to 1024 export entries, keyed by an opaque
Gateway UUID and project ID. Each entry expires one hour after creation. Expired
metadata may remain for one additional hour to return `expired`; a Gateway restart
loses all entries. At capacity, expired metadata is evicted first, so it cannot
block a new export; an evicted ID returns 404. This is not a shared or persistent evidence store. Multiple
Gateway replicas need request affinity for an export's lifetime.

Every status/content request rechecks the project and current source credentials.
Provider task IDs and download locations never leave the adapter/service boundary.
Creation starts at most one vendor task per request and is not automatically retried;
status GETs use the existing credential refresh/retry policy. Downloads propagate
request cancellation and expiration, close the upstream body, and never retry after
sending bytes. Stream failures abort the response instead of appending JSON.

NAD advertises `evidence_payload` and `evidence_file` through source capabilities.
The pilot restriction on downloading malware applies to verification practice,
not a blanket ban on file transport. Use supplied dumps or synthetic content.
PCAP references are provenance only; PCAP export is not advertised or implemented.
File streams use the export expiry deadline for body reads and HTTP writes, with
the source timeout retained for upstream response headers. JSON calls keep their
ordinary timeout. Cookie handling remains the existing temporary pilot mechanism.

The HTTP content operation streams the full response or a bounded byte slice.
IR MCP requests slices (16 KiB default, 64 KiB maximum) and returns Base64 with byte
offsets and EOF. It does not add binary evidence to IR snapshots or its database.
See [export contracts and case requests](evidence.md).

Wazuh keeps its event snapshot in OpenSearch, not in a process-local Gateway cache. Authenticated event cursors and a shared signing key allow continuation across Gateway replicas. Each incoming page still rechecks current project access and source credentials; a PIT does not confer project authorization. A lost/expired snapshot requires a new search. Source adapters own snapshot cleanup through the optional event cursor lifecycle interface.

Event cursor state is encrypted with AES-GCM as well as authenticated with HMAC. Purpose-separated keys are derived from `GATEWAY_CURSOR_SIGNING_KEY`; this also protects native sort values that can contain text redacted from canonical events. No extra key or project credential is required.
