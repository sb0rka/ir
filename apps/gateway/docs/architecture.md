# Gateway architecture

## Boundary

```text
HTTP client
  -> transport/http (auth, project allowlist, OpenAPI validation)
  -> service (credential cache, retry, fan-out, completeness)
  -> registry (provider selection by capability)
  -> provider adapter (typed vendor DTOs and fixed queries)
  -> normalization (canonical values, merge, sort)
```

Gateway owns no database, investigation, vendor URL, or credential supplied by a public request. Process configuration fixes URLs, TLS policy, and NAD store IDs. Project Secrets supply one cookie per project/source. The domain and service packages do not depend on HTTP, so a future MCP transport must use the same service boundary.

Providers register only implemented capabilities. The composition root constructs real `pt-maxpatrol-siem` and `pt-nad` adapters; an empty allowlist produces an empty registry. Mock and Sandbox providers are not registered and there is no generic proxy fallback.

Search calls fan out concurrently to the selected allowed providers. Each response carries `complete`, `truncated`, or `failed` source state. Gateway emits a cursor only when the provider confirmed a real continuation mechanism; it never invents a SIEM token or NAD continuation.

Event aggregation uses the same project-scoped fan-out and retry boundary, but returns source-local groups instead of canonical event records. Counts from different sources are never merged. A provider may support event search without implementing the narrower aggregation interface; mixed requests report that source as unsupported while retaining successful groups from other sources.

The bounded credential cache is keyed by `{project_id, source_code}` and serializes concurrent loads. A retryable network/timeout, `401/403`, or `5xx` failure invalidates only that entry, resolves its Secret again, and repeats the provider operation once. Redirects are rejected before credentials can move to another request.

## Object identity and context

Finding and session identity is `{source_code, source_instance, record_type, external_id}`. The required time range is replay provenance and is not part of identity. SIEM uses an empty source instance; NAD uses a configured store ID.

`Finding` and `Session` are first-class coarse objects. `Event`, `Entity`, and entity `Relation` remain granular evidence. Resolve retains a found root even when child context fails and marks that object `partial`; a missing root is not synthesized. Incident resolution includes correlation findings. Attack resolution includes its parent network session. Ordinary object responses contain metadata and evidence references. Explicit evidence exports stream payload or file bytes; cookies, password/NTLM material and full vendor JSON stay behind the adapter boundary. PCAP export is not enabled until its vendor contract is verified.

Canonical normalization covers IP, MAC, host, account, and hashes. Event entity mentions retain roles such as `src`, `dst`, `attacker`, and `victim`; flow direction never substitutes for attacker semantics.

## Temporary evidence exports

A bounded process-local registry owns up to 1024 export entries, keyed by an opaque
Gateway UUID and project ID. Each entry expires one hour after creation. Expired
metadata may remain for one additional hour to return `expired`; a Gateway restart
loses all entries. At capacity, expired metadata is evicted first, so it cannot
block a new export; an evicted ID returns 404. This is not a shared or persistent evidence store. Multiple
Gateway replicas need request affinity for an export's lifetime.

Every status/content request rechecks the project and current source allowlist.
Provider task IDs and download locations never leave the adapter/service boundary.
Creation starts at most one vendor task per request and is not automatically retried;
status GETs use the existing credential refresh/retry policy. Downloads propagate
request cancellation and expiration, close the upstream body, and never retry after
sending bytes. Stream failures abort the response instead of appending JSON.

NAD advertises `evidence_payload` and `evidence_file` through source capabilities.
PCAP references are provenance only; PCAP export is not advertised or implemented.
File streams use the export expiry deadline for body reads and HTTP writes, with
the source timeout retained for upstream response headers. JSON calls keep their
ordinary timeout. Cookie handling remains the existing temporary pilot mechanism.

The HTTP content operation streams the full response or a bounded byte slice.
IR MCP requests slices (16 KiB default, 64 KiB maximum) and returns Base64 with byte
offsets and EOF. It does not add binary evidence to IR snapshots or its database.
See [export contracts and case requests](evidence.md).
