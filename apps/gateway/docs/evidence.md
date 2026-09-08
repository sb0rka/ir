# NAD 2.1 / NAD 2.6 / SIEM 2.5: search and explicit evidence

## Status and boundary

Implemented: restricted NAD filters, SIEM incident creation range, alert/mail
context, full payload and selected-file ZIP export, HTTP streaming and MCP chunks.
PCAP references are retained; creation currently returns HTTP 422
`unsupported_capability`. Its start/status/download requests must first be captured
on the lab. The saved cases do not establish that chain.

No persistent binary storage, database migration or Dashboard change. Export
metadata is process-local for one hour; after a restart request a new export.
Unknown IDs and exports belonging to another project return the same 404.
Project/source access is checked again on every request. Vendor URLs, task IDs and
credentials are supplied by the server and are not accepted in the public body.

## Search grammar

`filter` accepts up to 4096 bytes and 16 nested parentheses. Allowed fields:
`src.ip`, `dst.ip`, `host.ip`, `host.port`, `app_proto`, `files.filename`,
`alert.pr`, `alert.msg`, `smb.rqs.create.filename`. Equality uses `==`; text wildcard
comparison uses `~`. Combine predicates with `AND`/`OR` or `&&`/`||`; AND binds
more tightly. Text values require double quotes. IPs and numbers may be unquoted.
No pipelines, arbitrary fields, single quotes/backslashes in values, or other
operators. Unsupported syntax returns an explicit source `invalid_request` error.

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
Each mail event carries `parent_session_id`. No attachment-to-mail relationship is
inferred solely from being in the same session. The Meterpreter alert exposes
malware family, signature metadata and a payload reference.

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

For a file, use `kind: "file"`, the session ref (`record_type: "nad_session"`) and
`object_id` from that session's file hint. The adapter verifies membership before
requesting only that flow/store/MD5 via `POST /api/v2/sources/getfile`. The returned
content is the vendor ZIP containing the selected extraction; it is not unpacked
or executed. Payload uses the exact alert's Base64 field without text conversion.

Creation returns 202 with `export_id`, `state`, `expires_at` and available metadata.
Poll `GET /api/v1/evidence/exports/{export_id}` / `gateway_get_evidence_export`.
States: `pending`, `ready`, `partial`, `failed`, `expired`. A partial extraction
has a safe error explanation and remains partial even after its bytes are read.
Transient status transport errors return an error without launching another task.

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

## Verification

`task gen` regenerates OpenAPI, Go server/client and TypeScript contracts.
`task build`, `task test`, `task vet`, `task typecheck` cover the implementation.
Tests replay selected fields from docs-internal PR #6 and synthetic task responses;
local HTTP tests cover scope, state, chunks and stream interruption. MCP tests call
the registered tools and reconstruct binary content across chunk boundaries.
These checks are not live NAD/SIEM E2E.

Release gate still requires an accessible lab: capture PCAP start/status/download,
implement that confirmed chain, then run all three cases through real Gateway HTTP
and MCP. Check shell bytes, extracted files, PCAP, mail/account metadata and SIEM
null-group drill-down. No live pass is claimed while this gate remains open.
