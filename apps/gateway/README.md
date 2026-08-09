# External tools Gateway

Gateway exposes normalized data from external security tools. Version `0.0.1` runs two contract-backed deterministic mock providers (MaxPatrol SIEM events and PT Sandbox artifact analysis); it does not call Investigations API and does not expose vendor payloads. PT NAD, MaxPatrol EDR, and PT Fusion stay unregistered until reviewed vendor contracts are available.

All temporary implementations and fixtures are isolated in `internal/adapters/mock`; application and HTTP code depend only on provider capability interfaces.

The default mock scenario contains 50,000 events and 5,000 host records spread over 90 days ending at `2026-07-23T12:00:00Z`. It is generated deterministically on startup, so IDs, timestamps, filters, and cursors are reproducible. Configure it with `MOCK_EVENT_COUNT`, `MOCK_ENDPOINT_COUNT`, and `MOCK_HISTORY_DAYS`; the supported maxima are 1,000,000 events, 100,000 host records, and 3,650 days.

## Run

```bash
docker compose -f docker-compose.dev.yml up -d --build gateway
```

Swagger is available at `http://localhost:8091/swagger`. Local Compose sets `AUTH_DISABLED=true`; protected routes still require `X-Project-ID`, for example `abcdef1234`. Outside local Compose, bearer authentication is enabled by default.

```bash
curl -s http://localhost:8091/api/v1/sources \
  -H 'X-Project-ID: abcdef1234'
```

Configuration for a provider uses `SOURCE_<CODE>_MODE`, `BASE_URL`, `CREDENTIAL_FILE`, `TIMEOUT_SEC`, and `TLS_CA_FILE` (hyphens become underscores). Only `mock` mode is implemented in `0.0.1`; selecting `proxy` stops startup with an explicit error.

Optional project allowlists are process configuration: `PROJECT_SOURCE_ALLOWLISTS={"abcdef1234":["maxpatrol-siem","pt-sandbox"]}`. Only registered sources are accepted; projects absent from the map can use every enabled source.

For a smaller local dataset:

```bash
MOCK_EVENT_COUNT=1000 MOCK_ENDPOINT_COUNT=100 MOCK_HISTORY_DAYS=30 go run ./apps/gateway/cmd/gateway
```

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](../../api/gateway/openapi.yaml).
