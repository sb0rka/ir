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

The bounded credential cache is keyed by `{project_id, source_code}` and serializes concurrent loads. A retryable network/timeout, `401/403`, or `5xx` failure invalidates only that entry, resolves its Secret again, and repeats the provider operation once. Redirects are rejected before credentials can move to another request.

## Object identity and context

Finding and session identity is `{source_code, source_instance, record_type, external_id}`. The required time range is replay provenance and is not part of identity. SIEM uses an empty source instance; NAD uses a configured store ID.

`Finding` and `Session` are first-class coarse objects. `Event`, `Entity`, and entity `Relation` remain granular evidence. Resolve retains a found root even when child context fails and marks that object `partial`; a missing root is not synthesized. Incident resolution includes correlation findings. Attack resolution includes its parent network session. Payloads, cookies, password/NTLM material, PCAP, downloaded files, and full vendor JSON stay behind the adapter boundary.

Canonical normalization covers IP, MAC, host, account, and hashes. Event entity mentions retain roles such as `src`, `dst`, `attacker`, and `victim`; flow direction never substitutes for attacker semantics.
