---
name: gateway-add-tool
description: Add or change an external security-tool provider, capability, mock normalizer, or vendor proxy client in ir/apps/gateway. Use for Gateway provider integrations while preserving its canonical API, credential boundary, registry model, and future MCP compatibility.
---

# Gateway: add a tool

Keep vendor details inside an adapter and keep the public contract canonical.

## Workflow

1. Read `apps/gateway/docs/architecture.md`, `apps/gateway/docs/providers.md`, and `references/provider-workflow.md`.
2. Decide whether the tool implements an existing capability. Add a capability only when no current interface expresses the operation.
3. Before changing an existing symbol, run the repository-required GitNexus impact analysis and report its risk.
4. Add a provider package with its descriptor, capability implementation, normalizer, and deterministic mock data.
5. Register the provider once in `internal/adapters/registry.go`. Do not add provider branches to HTTP handlers.
6. For real proxy mode, read configuration only from process config and use the shared safe HTTP client. Never accept a URL, token, or credential in a user request.
7. Add mapping, contract, partial-failure, and HTTP tests. Update OpenAPI only when the canonical capability changes.
8. Run Gateway generation, tests, vet, build, Docker smoke, skill validation, and GitNexus change detection.

Do not add raw proxy paths, `map[string]any` capability interfaces, reflection, dynamic plugins, MCP SDKs, or calls to Investigations API.
