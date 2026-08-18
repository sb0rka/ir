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

## Search SIEM events

```bash
curl -s http://localhost:8091/api/v1/events/search \
  -H 'X-Project-ID: abcdef1234' -H 'Content-Type: application/json' \
  -d '{"sources":["maxpatrol-siem"],"limit":50}'
```

## Unavailable capabilities

Entity lookup, endpoint search, and response-action catalogues have public contract placeholders but no registered provider in `0.0.1`. Do not call them until a reviewed provider is listed by `GET /api/v1/sources`.

## Analyze an artifact

```bash
curl -s http://localhost:8091/api/v1/artifact-analyses \
  -H 'X-Project-ID: abcdef1234' -H 'Content-Type: application/json' \
  -d '{"source":"pt-sandbox","artifact":{"name":"shell.php"}}'
```

## Errors

- `400`: malformed input, invalid ProjectID, or cursor mismatch.
- `401`: missing or invalid bearer token.
- `403`: no project role binding, no project source configuration, or a denied source.
- `404`: unknown source, endpoint, or analysis.
- `422`: the selected source lacks the capability or mock artifact.
- `502`: every selected source failed.
- `504`: request timeout.
