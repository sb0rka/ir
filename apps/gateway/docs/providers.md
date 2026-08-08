# Provider mappings

Only providers backed by a reviewed vendor operation are registered by the mock composition root. Their deterministic scenario data is first expressed as vendor DTOs and then passed through the same mapper intended for a real proxy. UI-only `position`, `style`, and `animated` fields from the source fixture are not part of any response.

Vendor DTOs live in `internal/adapters/proxy/<provider>/schema.go` and never enter the public Gateway contract. Black-box tests, contract samples, and provenance live under `tests/<provider>/`.

| Provider | Gateway capability | Vendor operation | Product version | Contract status |
| --- | --- | --- | --- | --- |
| MaxPatrol SIEM | events | `POST /api/events/v2/events` | SIEM 8.1 | Verified documentation and success JSON |
| MaxPatrol SIEM | entity lookup | — | — | Blocked: no reviewed operation or event predicate |
| PT Sandbox | artifact analysis | `POST /api/v1/analysis/createScanTask` | Sandbox 5.18 | Verified documentation and synchronous success JSON |
| PT NAD | events, entity lookup | — | — | Blocked: available Elasticsearch mapping is not a REST contract |
| MaxPatrol EDR | endpoints, response catalog | — | — | Blocked: customer API/OpenAPI response samples required |
| PT Fusion | entity lookup | — | — | Blocked: vendor states that OpenAPI exists, but the specification is not available in this repository |

## MaxPatrol SIEM (`maxpatrol-siem`)

Registered capability: event search. Raw and correlated records become canonical events through `proxy/maxpatrol.ToEventPage`; correlation metadata and selected dynamic fields stay in bounded attributes. Entity lookup is not registered until its contract blocker is resolved. Reference: [MaxPatrol SIEM event list API](https://help.ptsecurity.com/ru-RU/projects/siem/8.1/help/10836123659).

The deterministic mock uses a Gateway-only `token:offset` continuation to exercise canonical pagination. It is not claimed as the SIEM continuation protocol; a proxy implementation must confirm the vendor token exchange separately.

## PT NAD (`pt-nad`)

Not registered. The current `SessionDocument` draft is a selected subset of an Elasticsearch mapping, not a REST request or response schema. An OpenAPI document or anonymized HTTP JSON exchange is required before mapper or mock work continues. Reference: [PT NAD REST API entry point](https://help.ptsecurity.com/en-US/projects/nad/12.0/help/4750761739).

## MaxPatrol EDR (`maxpatrol-edr`)

Not registered. Endpoint inventory and response-action DTOs remain undefined until customer API/OpenAPI response samples are available. Gateway still has no action-execution route. Reference: [MaxPatrol EDR public API entry point](https://help.ptsecurity.com/ru-RU/projects/edr/9.1/help/7404729995).

## PT Sandbox (`pt-sandbox`)

Registered capability: artifact analysis. Known documents, scripts, executables, and extracted artifacts are expressed as the documented Sandbox 5.18 synchronous scan response and mapped through `proxy/sandbox.ToAnalysis`. The mock retains that response for Gateway `GetAnalysis`; it does not claim an undocumented vendor get-by-ID route. Reference: [Sandbox 5.18 scan](https://help.ptsecurity.com/ru-RU/projects/sb/5.18/developer/5040429451).

## PT Fusion (`pt-fusion`)

Not registered. The public documentation states that an OpenAPI specification exists, but a reviewed copy is required before defining the search DTO or mapper. Reference: [PT Fusion API](https://fusion.ptsecurity.com/docs/api/).
