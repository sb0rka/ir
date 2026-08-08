# PT Sandbox scan contract

- Product/version: PT Sandbox 5.18
- Operation: `POST /api/v1/analysis/createScanTask`
- Source: https://help.ptsecurity.com/ru-RU/projects/sb/5.18/developer/5040429451
- Retrieved: 2026-08-08
- Sample origin: request and synchronous success response published by the vendor
- Anonymization: file URI, file name, image ID, and one engine result were replaced or removed; field names and envelope structure were preserved
- Error fixture: the operation documents HTTP error codes but does not publish an error body; `error.json` is deliberately `null` instead of an invented envelope

The Gateway mock stores the mapped synchronous response for `GetAnalysis`; this does not claim an undocumented PT Sandbox get-by-ID endpoint.
