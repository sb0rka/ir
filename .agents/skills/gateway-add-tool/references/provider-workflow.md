# Provider workflow

## Existing capability

- Implement one or more interfaces from `internal/capability`.
- Return only types from `internal/domain`.
- Keep source code stable and lowercase with hyphens.
- Declare exactly the capabilities backed by non-nil implementations.
- Derive IDs with `domain.StableID`; use source + external ID for events and type + canonical value for entities.
- Put documented, consumer-useful vendor fields in bounded `attributes`; never retain the full vendor response.
- Add one constructor call to `internal/adapters/registry.go`.

## New capability

Add a focused interface, domain request/result types, application scenario, OpenAPI operation, and HTTP mapping. The application method must not accept HTTP-generated types. A future MCP transport must be able to call it directly.

## Mock

Mock records must form a coherent scenario across providers, use fixed timestamps and deterministic IDs, and exercise the real normalizer. Do not serve fixture JSON directly. Ignore presentation fields such as graph positions, styles, and animation.

## Real client

Use `mode`, `base_url`, `credential_file`, `timeout`, and `tls_ca_file`. Translate canonical request to vendor request, add authentication inside the adapter, call through `internal/proxy`, normalize the response, and return a structured error. Do not log secrets or raw responses.

## Verification

```bash
task gateway-gen
cd apps/gateway && go vet ./...
docker compose -f docker-compose.dev.yml up -d --build gateway
```

Smoke `readyz`, all five source descriptors, multi-source events, Fusion lookup, Sandbox analysis, EDR endpoints, and the response-action catalogue.
