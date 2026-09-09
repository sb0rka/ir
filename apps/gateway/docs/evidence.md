# NAD 2.1 / NAD 2.6 / SIEM 2.5: search and explicit evidence

## Status and boundary

Implemented: restricted NAD filters, SIEM incident creation range, alert/mail
context, full alert payload, HTTP streaming and MCP chunks.
NAD file extraction is disabled for the pilot: executables and attachments are
never downloaded. File names, hashes and session relations remain metadata only.
File requests return HTTP 422 `unsupported_capability` before any NAD request.
PCAP references are provenance only; use the dumps supplied separately by the
course. NAD PCAP export is not implemented or required for this pilot.

No persistent binary storage, database migration or Dashboard change. Export
metadata is process-local for one hour; after a restart request a new export.
Unknown IDs and exports belonging to another project return the same 404.
Project/source access is checked again on every request. Vendor URLs and
credentials are supplied by the server and are not accepted in the public body.

## Search grammar

`filter` accepts up to 4096 bytes and 16 nested parentheses. Allowed fields:
`src.ip`, `dst.ip`, `host.ip`, `host.port`, `app_proto`, `files.filename`,
`alert.pr`, `alert.msg`, `smb.rqs.create.filename`. Equality uses `==`; text wildcard
comparison uses `~`. Combine predicates with `AND`/`OR` or `&&`/`||`; AND binds
more tightly. Text values require double quotes. IPs and numbers may be unquoted.
No pipelines, arbitrary fields, single quotes/backslashes in values, or other
operators. Unsupported syntax returns an explicit source `invalid_request` error.
When all selected sources fail and one reports an invalid request, HTTP returns
400 with that validation error rather than hiding it behind `all_sources_failed`.

The adapter builds the same nested `EXISTS` predicates as the captured NAD UI,
inside the time-bounded parent flow for attack searches, before sorting/limiting.
Consequently, an alert filter in findings search selects matching flows and their
alerts, as in the case capture. The displayed result limit does not prefilter the
candidate set. Existing NAD truncation behavior remains: no unconfirmed cursor is
invented. Gateway cursors bind both new controls; empty controls retain the old
fingerprint.

## Case requests

All HTTP requests require the current bearer token and `X-Project-ID`. Use only
project-allowed sources and configured store IDs. Search fans out across those
stores; object refs returned by search contain the specific store.

### NAD 2.1

`POST /api/v1/findings/search` (same body for `gateway_search_findings`):

```json
{
  "sources": ["pt-nad"],
  "kinds": ["nad_attack"],
  "time_range": {"from": "2023-06-07T21:00:00Z", "to": "2023-06-08T20:59:59Z"},
  "filter": "alert.msg ~ \"*PSEXEC*\" OR smb.rqs.create.filename ~ \"*PSEXESVC*\"",
  "limit": 50
}
```

`POST /api/v1/sessions/search` (same body for `gateway_search_sessions`):

```json
{
  "sources": ["pt-nad"],
  "time_range": {"from": "2023-06-07T21:00:00Z", "to": "2023-06-08T20:59:59Z"},
  "filter": "host.port == 2222 && src.ip == 192.168.25.202 && dst.ip == 192.168.214.203 && alert.pr == 2",
  "limit": 50
}
```

Resolve the returned session/finding. Select the shell-banner alert's payload
reference and export it. The fixture preserves the exact original Base64 banner;
metadata previews are not a substitute for reading the full export.

### NAD 2.6

Use `time_range` from `2023-06-10T09:50:00Z` through `2023-06-10T09:59:59Z`.
Search sessions separately with these filters (JSON string values):

```json
"files.filename == \"(ChromeUpdate.exe)\""
```

```json
"app_proto == \"imap\" && host.ip == 192.168.25.190"
```

The parentheses in `(ChromeUpdate.exe)` are part of the captured filename. To find
variants, use `files.filename ~ "*ChromeUpdate.exe*"` instead.
Session `mail_hints` contain from/to/subject/date; context adds `network.mail` events,
email entities and the account from `credentials.login` or `credentials.user`.
The date is read from either the flat field or the MIME `Date` header (including
the parallel `headers.key` / `headers.value` representation). Quoted account
strings are unquoted before normalization. Embedded files may omit `parent` when
the enclosing flow ID matches the requested session; explicitly foreign or invalid
children are excluded and the retained session is marked partial.
Each mail event carries `parent_session_id`. No attachment-to-mail relationship is
inferred solely from being in the same session. The Meterpreter alert exposes
malware family, signature metadata and a payload reference.

### Live verification on 2026-09-09

Against stores 19 and 23, both Gateway HTTP and IR MCP returned two PSEXEC findings,
one TCP/2222 shell session, one ChromeUpdate session and three IMAP sessions.
The target IMAP context retained five files, its sender/recipient/subject/date and
normalized account. A limit-one IMAP query still reported three vendor matches.

Alert payloads were compared between HTTP and concatenated MCP Base64 chunks:

| Evidence | Original bytes | SHA-256 |
| --- | ---: | --- |
| Shell banner | 116 | `3d9c61d64e2741fdd8bb3b9124d9169ae0932bf2d070da7cdd46bc304789905f` |
| Meterpreter payload | 3000 | `b7fa96cfcbdb0e18fcaae8d8747b92de97978bc21120161abe6cba5cf814f268` |

The production Gateway and IR API images were used with JWT validation enabled.
A separate local test helper delivered `.env` cookies through the Secrets API
contract; production Sb0rka secret storage/authentication was not tested by this
run. Binary exports were kept only as local test artifacts, outside IR storage.

The earlier run also exercised file exports. That behavior has since been removed
from the pilot; those historical results are not a supported capability or an
instruction to repeat file downloads. Current checks must verify rejection of
file exports and preservation of file metadata without downloading file content.

### SIEM 2.5

Use `POST /api/v1/findings/search` / `gateway_search_findings` with
`sources: ["pt-maxpatrol-siem"]` and `kinds: ["siem_incident"]`.
`time_range` specifies incident detection time; optional `created_at_range` specifies
creation time independently. Use the actual lab creation dates, which may differ
from event dates after a case is imported. Both ranges require `from < to`.

Continue with the existing `gateway_aggregate_events` and `gateway_search_events`.
Pass the same `filter`, ordered `group_by` and the selected group's `group_values`.
A null group is JSON `null`, not an empty string or the string `"null"`. Existing
aggregation/null-group tests remain part of the regression suite.

## Export and read

Pass one complete evidence reference returned by a finding/session to
`POST /api/v1/evidence/exports` or `gateway_create_evidence_export`:

```json
{
  "kind": "payload",
  "ref": {
    "source_code": "pt-nad",
    "source_instance": "19",
    "record_type": "nad_attack",
    "external_id": "vRwB4Z4BaLX3hldUvkZ1",
    "time_range": {"from": "2023-06-07T21:00:00Z", "to": "2023-06-08T20:59:59Z"}
  }
}
```

Payload uses the exact alert's Base64 field without text conversion. Do not submit
file IDs for extraction: NAD supports `kind: "payload"` only. File and PCAP requests
return `unsupported_capability`; case PCAP dumps are supplied outside Gateway.

Creation returns 202 with `export_id`, `state`, `expires_at` and available metadata.
Poll `GET /api/v1/evidence/exports/{export_id}` / `gateway_get_evidence_export`.
The shared lifecycle has `pending`, `ready`, `partial`, `failed`, `expired` states.
NAD payload is ready at successful creation and expires after one hour; it starts
no vendor extraction task. Partial responses, when present, remain marked partial.

`GET /api/v1/evidence/exports/{export_id}/content` streams the complete bytes with
Content-Type, Content-Disposition and no-store headers. Optional `offset` and
`limit` return a bounded slice with `X-Evidence-EOF`. Pending/failed content returns
409, expired content 410, and an offset beyond EOF 416. A stream interrupted after
headers aborts the connection; discard that partial download before retrying.

MCP example (`gateway_read_evidence_content`):

```json
{"export_id": "11111111-2222-4333-8444-555555555555", "offset": 0, "limit": 16384}
```

The response contains `encoding: "base64"`, `content`, `size`, `offset`,
`next_offset` and `eof`. Concatenate decoded chunks, advancing to `next_offset`
until `eof: true`. Default 16 KiB, maximum 64 KiB. Base64 avoids UTF-8 split loss.
Each read reopens the vendor object and skips preceding bytes: no evidence bytes
are cached, and large exports cost repeated upstream reads. Reuse the same export
only while it remains valid. Payload detail retains the existing bounded JSON
response limit; oversized responses fail explicitly rather than returning a
silently truncated payload.

Payload reads are repeated upstream requests; no binary storage or cache is
introduced. Source discovery advertises `evidence_payload` for NAD, never
`evidence_file`. At registry capacity, expired metadata can be evicted before its
additional retention hour, returning 404 thereafter.

## Verification

`task gen` regenerates OpenAPI, Go server/client and TypeScript contracts.
`task build`, `task test`, `task vet`, `task typecheck` cover the implementation.
Tests replay selected fields from docs-internal PR #6 and synthetic payload bytes;
local HTTP tests cover scope, state, chunks and stream interruption. MCP tests call
the registered tools and reconstruct binary content across chunk boundaries.
These checks are not live NAD/SIEM E2E.

For the agreed NAD pilot, PCAP is supplied separately by the course; exporting it
from NAD is not a release gate. The NAD HTTP/MCP results above cover the implemented
search, context and payload operations. They do not establish live SIEM 2.5
coverage; SIEM creation/detection ranges and null-group drill-down require a separate
live check before claiming that case complete.
