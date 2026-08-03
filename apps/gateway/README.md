# External tools Gateway

Gateway exposes normalized data from external security tools. Version `0.0.1` runs five deterministic mock providers; it does not call Investigations API and does not expose vendor payloads.

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

Optional project allowlists are process configuration: `PROJECT_SOURCE_ALLOWLISTS={"abcdef1234":["maxpatrol-siem","pt-nad"]}`. Projects absent from the map can use every enabled source.

See [architecture](docs/architecture.md), [provider mappings](docs/providers.md), and the [OpenAPI contract](api/openapi.yaml).
