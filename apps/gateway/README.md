# External tools Gateway

Gateway exposes normalized, object-centric data from connected security products, including PT and Wazuh. The primary objects are MaxPatrol SIEM incidents and correlation firings plus PT NAD attacks and network sessions. Raw events and entities remain available for drill-down and investigation graph material. Gateway owns no investigation data and never exposes a raw vendor proxy.

The compiled source catalog registers `pt-maxpatrol-siem`, `pt-nad` and `wazuh`. Each definition owns its server settings, adapter factory and project credential decoders; see the [source catalog workflow](docs/providers.md#source-catalog). There is no mock mode, scenario registry, generated fixture runtime, dummy provider, or Sandbox fallback.

## Run

From the repository root:

```bash
docker compose --profile apps up -d --build gateway
```

Swagger is available at `http://localhost:8091/swagger`. The browser calls Gateway through IR API. Protected requests carry a Gateway JWT, a separate API JWT with exactly `secrets.read` and `secrets.reveal` in `X-IR-Config-Token`, and `X-Project-ID`. The two identities must match; live grants, session and current user RBAC remain mandatory.

Agent requests carry the project key `s0_ir_agent_…` in `Authorization: Bearer` and `X-Project-ID`, without `X-IR-Config-Token`. Gateway verifies the active SHA-256 digest in the IR database on every request and decrypts that project's saved source configuration. Agent requests require neither a user session nor Auth/Platform backchannel calls. Rotation immediately invalidates the previous key.

Enable agent access with `DATABASE_URI` pointing to the existing IR database and `IR_GATEWAY_CONFIGURATION_KEY` containing the same base64-encoded 32-byte AES key as IR API. Both settings must be supplied together. IR synchronizes the encrypted source configuration when it is saved or reloaded; direct edits in Platform Secrets become available to agents after reloading through IR.

Sources are registered from server configuration. A project can use a registered source when its current `IR/SOURCE_CONFIGURATION` contains valid source credentials. No manual project/source map is needed. Empty source settings start Gateway with an empty registry and `/api/v1/sources` returns `{"items":[]}`.

Example process configuration to enable the backends:

```bash
SOURCE_PT_MAXPATROL_SIEM_BASE_URL='https://mp-siem.edtechlab.local'
SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL='https://mp-siem.edtechlab.local:8887'
SOURCE_PT_NAD_BASE_URL='https://pt-nad.edtechlab.local'
SOURCE_PT_NAD_STORE_IDS='19,20'
SOURCE_WAZUH_BASE_URL='https://localhost:9200'
SOURCE_WAZUH_SKIP_TLS_VERIFY='true' # only for this self-signed lab endpoint
# Generate once, store outside Git, and share across Gateway replicas:
GATEWAY_CURSOR_SIGNING_KEY='<base64-encoded 32-byte key>'
```

SIEM requires both backend URLs. NAD requires its URL and unique positive store IDs. Partial settings fail startup. Store IDs remain process-owned; requests may replay only a configured source instance. URLs and store IDs in project JSON are ignored.

For the existing local simulator, use HTTP: both SIEM URLs point to
`http://192.168.44.129:8099`, NAD points to `http://192.168.44.129:8100`.
Set NAD store IDs from its `/api/v2/sources` catalog. IR API in Compose connects
to `GATEWAY_BASE_URL=http://gateway:8091`; `localhost:8091` is for running IR API
on the host. Normal Compose starts without OpenVPN and connects directly to
these server-configured URLs.

IR reads one ordinary JSON secret `IR/SOURCE_CONFIGURATION` from sb0rka and interprets its source array. IR attaches the existing `source:ir` resource tag when saving:

```json
[
  {"name":"pt-maxpatrol-siem","CorePortalCookie":"…","idsrv.session":"…","idsrv":"…"},
  {"name":"pt-nad","sessionid":"…","csrftoken":"…"},
  {"name":"wazuh","username":"…","password":"…"}
]
```

Gateway selects only known credential fields from the matching source object. PT uses cookies; Wazuh uses Basic Authorization. Wazuh requires nonempty username/password strings, preserves their spaces, and rejects control characters and a colon in the username. It has no ENV credential fallback. Missing, incomplete or invalid credential fields hide that source for the project and deny explicit requests before any vendor connection. Changes take effect on the next request, without restarting Gateway. Cookie format validation does not establish that credentials are still accepted by the vendor; probes and operation responses determine that.

An explicitly present empty string is sent as `CookieName=`; it is distinct from
an absent field. NAD also preserves an explicitly empty CSRF header. The upstream
decides whether these credentials are accepted. Dashboard lists names from the
saved JSON even when Gateway cannot connect to them, and refreshes their statuses
after saving or on demand.

Plaintext credentials are not cached between requests. Each request uses one JSON snapshot across availability checks, fan-out and retries. Internal JWTs and unrelated JSON keys are never sent to vendors. Removing credentials also denies access to already-ready evidence exports. Backend status alone has a short cache and never bypasses current credential checks.

Outbound TLS requires TLS 1.2 or newer and redirects are not followed. `GATEWAY_SKIP_TLS_VERIFY` defaults to `false`; configure provider CA files for private certificates. `/healthz` is process-local. `/api/v1/sources` probes configured backends or NAD stores for the currently available sources and reports `online`, `degraded`, or `offline` through a short status cache.

Development and contract tests use reviewed, sanitized captures from `docs-internal/pt-cases` and local HTTP fixtures. They do not contact real SIEM/NAD hosts.

## Advanced event search

`POST /api/v1/events/search` accepts the source-independent `sources`, `time_range`, `entities`, `limit`, and `cursor` fields. For `pt-maxpatrol-siem` and `wazuh`, it additionally accepts:

- `filter` — a PDQL predicate only, without `filter(...)`, pipelines, comments, or control characters;
- `columns` — allowlisted event fields to fetch;
- `sort` — ordered `{ "field": "time", "direction": "asc" | "desc" }` rules;
- `group_by` and aligned `group_values` — the selected source group path. A `null` item selects the source null group.

Gateway builds the vendor query itself and always fetches the canonical identity fields required to return normalized events. Unknown fields and unsafe predicate syntax are rejected before an outbound call. These controls are source-specific: a request that selects `pt-nad` with any of them returns a source error instead of silently ignoring the options.

## Event aggregation

`POST /api/v1/events/aggregate` returns source-local `{source_code, values, count}` groups for a required `time_range` and `group_by`. It accepts the same bounded entity conditions and predicate syntax as event search. Group sorting is limited to canonical `count` or a requested `group_by` field; the default is `count desc`. `limit` is applied independently to each source and there is no aggregation cursor or cross-source count merge.

Wazuh uses PIT-backed composite aggregation, including null groups, and selects global top-N only after enumerating the groups within the source timeout. MaxPatrol uses its reviewed `/api/events/v3/events/aggregation` operation. The adapter owns the required `select`, `group`, count alias, tie-break sorting, and limit pipeline. PT NAD remains an event source for normalized search and context, but reports `unsupported_event_aggregation` for this narrower operation. To drill into one returned group, call `/api/v1/events/search` with the same `group_by` and aligned `group_values`.

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](../../api/gateway/openapi.yaml).

## NAD case context and evidence

Findings/sessions accept restricted NAD `filter` predicates; SIEM incidents also
accept `created_at_range`. Session details include mail metadata and account hints.
Full payload and selected-file bytes use explicit evidence exports and bounded MCP
reads. See [case requests, lifecycle and verification](docs/evidence.md).

File download support is retained. Pilot checks use supplied dumps or synthetic
content and do not download malware or suspicious attachments. PCAP supplied by
the course is analyzed separately; NAD session-PCAP export remains unsupported.
Local contract tests do not establish live case coverage.

## Event pagination

Set `GATEWAY_CURSOR_SIGNING_KEY` to a Base64-encoded 32-byte key before enabling event sources. Empty registries do not require it. Cursor payloads are versioned, encrypted and authenticated, bound to the project, query and server source configuration; they contain neither events nor credentials. All replicas must use the same key. Old event cursors become invalid after this upgrade; findings/session cursors are unchanged.

The merger replays a partly consumed provider page with its original size and position, and advances only after that page has been delivered. This retains source tails across mixed pages and changes to the public page size. Per-source totals and completion states survive continuation. Numeric sort fields are compared numerically, with stable event identity resolving ties. A failed source keeps its last delivered position for retry; page ordering covers the sources that responded, and recovery can introduce earlier events.

Wazuh uses PIT and `search_after` without an offset ceiling. The PIT and signed cursor expire after five minutes without continuation. Completion closes the PIT; abandoned contexts expire automatically. Expired/lost PIT returns `invalid_cursor`, requiring a new search. PT retains its existing consistency guarantees; NAD has no confirmed native event continuation and retains `truncated` after its available window is consumed.

For Wazuh-native filter examples and the OpenSearch 2.19 sort compatibility cost, see [Wazuh mapping](docs/providers.md#wazuh-wazuh). A proxy-enabled installation must include the private Indexer host in `NO_PROXY`; vendor routing still uses the shared HTTP client's proxy policy.

Event cursor state is encrypted with AES-GCM as well as authenticated with HMAC. Purpose-separated keys are derived from `GATEWAY_CURSOR_SIGNING_KEY`; this also protects native sort values that can contain text redacted from canonical events. No extra key or project credential is required.

Combined event-query rules are in [Event query](docs/architecture.md#event-query).
