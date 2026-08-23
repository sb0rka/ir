# External tools Gateway

Gateway exposes normalized data from external security tools. Version `0.0.1` runs two contract-backed deterministic mock providers (MaxPatrol SIEM events and PT Sandbox artifact analysis) and checks the MaxPatrol account through its real `userinfo` operation. It does not call Investigations API and does not expose vendor payloads. PT NAD, MaxPatrol EDR, and PT Fusion stay unregistered until reviewed vendor contracts are available.

All temporary implementations and fixtures are isolated in `internal/adapters/mock`; service and HTTP code depend only on provider capability interfaces.

The default mock scenario contains 50,000 events and 5,000 host records spread over 90 days ending at `2026-07-23T12:00:00Z`. It is generated deterministically on startup, so IDs, timestamps, filters, and cursors are reproducible. Configure it with `MOCK_EVENT_COUNT`, `MOCK_ENDPOINT_COUNT`, and `MOCK_HISTORY_DAYS`; the supported maxima are 1,000,000 events, 100,000 host records, and 3,650 days.

## Run

```bash
docker compose -f docker-compose.dev.yml up -d --build gateway
```

Swagger is available at `http://localhost:8091/swagger`. Bearer authentication is enabled by default and uses the configured issuer, audience, key ID, token type, and Ed25519 key. `ACCESS_TOKEN_PRIVATE_KEY`/`_FILE_PATH` derives the verification key from the same PKCS#8 key used by Auth; `ACCESS_TOKEN_PUBLIC_KEY`/`_FILE_PATH` remains a fallback. Protected routes also require `X-Project-ID`, for example `abcdef1234`; `AUTH_DISABLED=true` is reserved for isolated development checks.

```bash
curl -s http://localhost:8091/api/v1/sources \
  -H 'X-Project-ID: abcdef1234'
```

Configuration for a provider uses `SOURCE_<CODE>_MODE`, `BASE_URL`, `CREDENTIAL_FILE`, `TIMEOUT_SEC`, and `TLS_CA_FILE` (hyphens become underscores). Only `mock` mode is implemented in `0.0.1`; selecting `proxy` stops startup with an explicit error.

Project allowlists are process configuration: `PROJECT_SOURCE_ALLOWLISTS={"abcdef1234":["maxpatrol-siem","pt-sandbox"]}`. Only registered sources are accepted; projects absent from the map are denied. JWT validation and this allowlist are the Gateway access boundary; Gateway has no database or role-binding lookup.

`SB0RKA_API_BASE_URL` points to the Sb0rka API. For `maxpatrol-siem`, Gateway resolves `DEMO_PT_SIEM_BASE_URL` and `DEMO_PT_COOKIE` from the selected project's Secrets using the already validated caller token. Credentials are cached in memory, never returned or logged, and reloaded once after a network/timeout, authentication, or upstream `5xx` failure. `GET /api/v1/sources/{source}/account/userinfo` forces a reload; `GET /api/v1/sources` actively probes account-capable sources. In `mock` mode a missing or failed real account probe does not disable the deterministic mock capabilities: the source remains `online` with `mode=mock`, while the explicit `userinfo` request still returns its configuration or upstream error.

For a smaller local dataset:

```bash
AUTH_DISABLED=true SB0RKA_API_BASE_URL=http://localhost:8080 \
  PROJECT_SOURCE_ALLOWLISTS='{"abcdef1234":["maxpatrol-siem","pt-sandbox"]}' \
  MOCK_EVENT_COUNT=1000 MOCK_ENDPOINT_COUNT=100 MOCK_HISTORY_DAYS=30 \
  go run ./apps/gateway/cmd/gateway
```

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](../../api/gateway/openapi.yaml).
