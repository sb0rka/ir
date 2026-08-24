# External tools Gateway

Gateway exposes normalized, object-centric data from the connected PT products. The primary objects are MaxPatrol SIEM incidents and correlation firings plus PT NAD attacks and network sessions. Raw events and entities remain available for drill-down and investigation graph material. Gateway owns no investigation data and never exposes a raw vendor proxy.

The registered source codes are exactly `pt-maxpatrol-siem` and `pt-nad`. There is no mock mode, scenario registry, generated fixture runtime, dummy provider, or Sandbox fallback.

## Run

```bash
docker compose -f docker-compose.dev.yml up -d --build gateway
```

Swagger is available at `http://localhost:8091/swagger`. Protected routes require a valid bearer token and `X-Project-ID`. The project must exist in `PROJECT_SOURCE_ALLOWLISTS`; an absent project is forbidden. An empty or missing allowlist starts Gateway without external sources.

Demo process configuration:

```bash
PROJECT_SOURCE_ALLOWLISTS='{"ee995bf220":["pt-maxpatrol-siem","pt-nad"]}'
SOURCE_PT_MAXPATROL_SIEM_BASE_URL='https://mp-siem.edtechlab.local'
SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL='https://mp-siem.edtechlab.local:8887'
SOURCE_PT_NAD_BASE_URL='https://pt-nad.edtechlab.local'
SOURCE_PT_NAD_STORE_IDS='19,20'
```

Store IDs are unique positive integers and are process-owned. Requests may only replay a source instance returned by Gateway, and that instance must remain in this allowlist. An allowlisted source with missing URL or store configuration is a startup error.

Project Secrets provide only the rotating credentials:

- `DEMO_PT_SIEM_COOKIE` for `pt-maxpatrol-siem` — combined header:
  - Events API (`POST /api/events/v3/events`): `e`, `idsrv.session`, `idsrv`;
  - Incident Read Model (`:8887`): `IncidentManagementPortalCookie`;
- `DEMO_PT_NAD_COOKIE` for `pt-nad` — `sessionid=<value>; csrftoken=<value>` (both required).

Cookies are project/source-scoped, bounded in memory, and reloaded once after a network/timeout, `401/403`, or `5xx` failure. They are sent only in the `Cookie` header and are never logged. Missing Secrets make that source offline for the request without preventing startup.

Outbound TLS requires TLS 1.2 or newer and redirects are not followed. `GATEWAY_SKIP_TLS_VERIFY` defaults to `false`; the demo must explicitly set it to `true` or configure the provider CA files. `/healthz` is process-local. `/api/v1/sources` probes both SIEM backends or every NAD store and reports `online`, `degraded`, or `offline` through a short cache.

Development and contract tests use reviewed, sanitized captures from `docs-internal/pt-cases` and local `httptest` servers. They do not contact the OpenVPN-only hosts.

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](../../api/gateway/openapi.yaml).
