# External tools Gateway

Gateway exposes normalized, object-centric data from connected security products.
The primary objects are MaxPatrol SIEM incidents and correlation firings, PT NAD
attacks and network sessions, and Wazuh Indexer alerts as canonical events.
Raw events and entities remain available for drill-down and investigation graph
material. Gateway owns no investigation data and never exposes a raw vendor proxy.

The registered source codes are `pt-maxpatrol-siem`, `pt-nad`, and `wazuh`. There
is no mock mode, scenario registry, generated fixture runtime, dummy provider, or
Sandbox fallback.

## Run

```bash
task apps:up          # ir-api + gateway (LAN / direct network)
task vpn:up           # optional OpenVPN client only (PT lab)
```

Or with compose directly:

```bash
docker compose --profile apps up -d --build
docker compose --profile vpn up -d --build openvpn
```

Host-run:

```bash
set -a && source .env && set +a
cd apps/gateway && go run ./cmd/gateway
```

Swagger is available at `http://localhost:8091/swagger`. Protected routes require a valid bearer token and `X-Project-ID`. The project must exist in `PROJECT_SOURCE_ALLOWLISTS`; an absent project is forbidden. An empty or missing allowlist starts Gateway without external sources.

Example process configuration (placeholders only):

```bash
PROJECT_SOURCE_ALLOWLISTS='{"<project-id>":["pt-maxpatrol-siem","pt-nad","wazuh"]}'
SOURCE_PT_MAXPATROL_SIEM_BASE_URL='https://mp-siem.example'
SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL='https://mp-siem.example:8887'
SOURCE_PT_NAD_BASE_URL='https://pt-nad.example'
SOURCE_PT_NAD_STORE_IDS='19,20'
SOURCE_WAZUH_BASE_URL='https://wazuh-indexer.example:9200'
SOURCE_WAZUH_USERNAME='admin'
SOURCE_WAZUH_PASSWORD='...'
SOURCE_WAZUH_INSECURE_SKIP_VERIFY='true'
```

Store IDs are unique positive integers and are process-owned. Requests may only replay a source instance returned by Gateway, and that instance must remain in this allowlist. An allowlisted source with missing URL or store configuration is a startup error.

Project Secrets provide rotating credentials for PT sources:

- `DEMO_PT_SIEM_COOKIE` for `pt-maxpatrol-siem` — combined header:
  - Events API (`POST /api/events/v3/events`): `CorePortalCookie`, `idsrv.session`, `idsrv`;
  - Incident Read Model (`:8887`): `IncidentManagementPortalCookie`;
- `DEMO_PT_NAD_COOKIE` for `pt-nad` — `sessionid=<value>; csrftoken=<value>` (both required).

`wazuh` uses process-level Basic Auth from `SOURCE_WAZUH_USERNAME` /
`SOURCE_WAZUH_PASSWORD` instead of project Secrets (demo credential mode).

Cookies are project/source-scoped, bounded in memory, and reloaded once after a network/timeout, `401/403`, or `5xx` failure. They are sent only in the `Cookie` header and are never logged. Missing Secrets make that source offline for the request without preventing startup.

Outbound TLS requires TLS 1.2 or newer and redirects are not followed. `GATEWAY_SKIP_TLS_VERIFY` defaults to `false` for PT providers; Wazuh uses its own `SOURCE_WAZUH_INSECURE_SKIP_VERIFY`. `/healthz` is process-local. `/api/v1/sources` probes configured backends and reports `online`, `degraded`, or `offline` through a short cache.

Development and contract tests use reviewed, sanitized captures and local `httptest` servers. They do not contact the OpenVPN-only hosts.

## Advanced event search

`POST /api/v1/events/search` accepts the source-independent `sources`, `time_range`, `entities`, `limit`, and `cursor` fields. For `pt-maxpatrol-siem` and `wazuh`, it additionally accepts:

- `filter` — a PDQL predicate only, without `filter(...)`, pipelines, comments, or control characters;
- `columns` — allowlisted event fields to fetch;
- `sort` — ordered `{ "field": "time", "direction": "asc" | "desc" }` rules;
- `group_by` and aligned `group_values` — the selected group path. A `null` item selects the source null group.

Gateway builds the vendor query itself and always fetches the canonical identity fields required to return normalized events. Unknown fields and unsafe predicate syntax are rejected before an outbound call for that source. These controls are source-specific: a request that selects `pt-nad` with any of them returns a source error instead of silently ignoring the options. MaxPatrol and Wazuh each validate their own field allowlists (Wazuh uses native Indexer paths, not MaxPatrol aliases), so a mixed search can succeed for one SIEM while the other reports `source_errors`.

## Event aggregation

`POST /api/v1/events/aggregate` returns source-local `{source_code, values, count}` groups for a required `time_range` and `group_by`. It accepts the same bounded entity conditions and predicate syntax as event search. Group sorting is limited to canonical `count` or a requested `group_by` field; the default is `count desc`. `limit` is applied independently to each source and there is no aggregation cursor or cross-source count merge.

MaxPatrol uses its reviewed `/api/events/v3/events/aggregation` operation. Wazuh uses Indexer terms/composite aggregations and a synthetic `correlation_type` filters aggregation. PT NAD remains an event source for normalized search and context, but reports `unsupported_event_aggregation` for this narrower operation. To drill into one returned group, call `/api/v1/events/search` with the same `group_by` and aligned `group_values`.

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](../../api/gateway/openapi.yaml).
