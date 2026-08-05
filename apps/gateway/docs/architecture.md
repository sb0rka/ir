# Gateway architecture

## Boundary

Gateway is a stateless adapter service for external security products. It owns no investigations, does not call Investigations API, does not execute EDR response actions, and has no MCP transport in `0.0.1`.

```text
HTTP client
  -> transport/http (auth, ProjectID, CORS, OpenAPI)
  -> application (fan-out, timeouts, pagination)
  -> registry (provider selection by capability)
  -> adapter (mock or vendor implementation)
  -> normalization (canonical values, merge, sort)
```

The domain and application packages do not depend on HTTP. A future `transport/mcp` must call the same application service.

## Provider contract

Providers register a descriptor and only the capability interfaces they implement. Adding another provider for an existing capability changes the provider package and registry construction, not the router or OpenAPI.

Capability interfaces in `internal/capability` are the boundary between the Gateway and provider implementations. All temporary data, scenario generation, fixtures, and mock providers live under `internal/adapters/mock`. The composition root imports only `mock.NewRegistry`; domain, application, registry, normalization, and HTTP transport do not import the mock package.

A real client belongs under `internal/adapters/proxy/<provider>` and implements the same capability interfaces. Replacing the temporary implementation requires changing the composition root and deleting `internal/adapters/mock`; it does not change the application or public API.

Requests without `sources` fan out to all registered providers with the requested capability. Calls run concurrently with a 10-second source timeout inside a 15-second request timeout. Successful data is returned with `source_errors` when only part of the fan-out fails; failure of every selected source returns `502`.

`X-Project-ID` is validated before dispatch, included in request logs, and selects the process-owned source allowlist. An omitted `sources` field expands only to sources allowed for that project; explicitly requesting a denied source returns `403`.

Event cursors are base64url-encoded, stateless state. They contain a request fingerprint and one opaque continuation per source. A cursor cannot be reused with different filters.

## Data boundary

Adapters translate vendor data into `Event`, `Entity`, `Relation`, `Artifact`, `Analysis`, `Endpoint`, `Verdict`, and `Provenance`. `attributes` contains only mapped fields needed by consumers. Full vendor responses, credentials, base URLs, graph UI layout, and executable response actions are not exposed.

Entity identity is `type + canonical value`; event identity is `source + external ID`. Stable UUIDs are derived from those keys. Results are sorted by event time, source, and external ID.

## Real proxy mode

Provider configuration is process-owned. User requests never accept credentials or vendor URLs. The shared HTTP factory enforces HTTP(S), rejects URL userinfo/query/fragment, requires positive timeouts, supports a credential file and custom CA, and sets TLS 1.2 as the minimum.

`proxy` mode intentionally fails during startup until that provider has a real client. A vendor client must translate canonical requests, add authentication, call the vendor, normalize the response, and return a structured source error.
