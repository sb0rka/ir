---
name: gateway-use-http
description: Query external security tools through the normalized Gateway HTTP API in ir/apps/gateway. Use for source discovery, multi-source event search, entity reputation lookup, artifact analysis, endpoint search, or listing EDR response actions.
---

# Gateway HTTP API

Read `references/http-api.md` for request examples and response behavior.

## Rules

- Use Gateway for external tools only; use the Investigations API directly for investigations and graphs.
- Send `Authorization: Bearer <JWT>` unless the local Compose bypass is active.
- Always send a 10–12 character lowercase hexadecimal `X-Project-ID`.
- The authenticated subject must have a role binding in that project, and the project must have a configured source allowlist.
- Omit `sources` to call every enabled provider with the requested capability; specify source codes to constrain fan-out.
- Treat `source_errors` as partial failure. Retry only entries with `retryable: true`.
- Follow `next_cursor` without decoding or changing it, and keep all filters identical.
- Use the OpenAPI document at `/openapi.yaml` as the contract. Do not depend on vendor-shaped fields in `attributes` unless their provider mapping documents them.

Gateway lists EDR actions but does not execute them.
