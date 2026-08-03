# HTTP quick reference

Base URL: `http://localhost:8091`. Headers below are required on `/api/v1/*` routes:

```text
Authorization: Bearer <access token>
X-Project-ID: abcdef1234
Content-Type: application/json
```

Local Compose disables bearer verification explicitly but still requires ProjectID.

## Discover sources

```bash
curl -s http://localhost:8091/api/v1/sources -H 'X-Project-ID: abcdef1234'
```

## Search SIEM and NAD events

```bash
curl -s http://localhost:8091/api/v1/events/search \
  -H 'X-Project-ID: abcdef1234' -H 'Content-Type: application/json' \
  -d '{"sources":["maxpatrol-siem","pt-nad"],"limit":50}'
```

## Look up reputation

```bash
curl -s http://localhost:8091/api/v1/entities/lookup \
  -H 'X-Project-ID: abcdef1234' -H 'Content-Type: application/json' \
  -d '{"sources":["pt-fusion"],"entity":{"type":"ip","value":"10.125.11.90"}}'
```

## Analyze an artifact

```bash
curl -s http://localhost:8091/api/v1/artifact-analyses \
  -H 'X-Project-ID: abcdef1234' -H 'Content-Type: application/json' \
  -d '{"source":"pt-sandbox","artifact":{"name":"shell.php"}}'
```

## Search endpoints and list actions

POST `/api/v1/endpoints/search` with `{"sources":["maxpatrol-edr"]}`. Use the returned `external_id` in GET `/api/v1/sources/maxpatrol-edr/endpoints/{external_id}/response-actions`.

## Errors

- `400`: malformed input, invalid ProjectID, or cursor mismatch.
- `401`: missing or invalid bearer token.
- `404`: unknown source, endpoint, or analysis.
- `422`: the selected source lacks the capability or mock artifact.
- `502`: every selected source failed.
- `504`: request timeout.
