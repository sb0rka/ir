# MaxPatrol SIEM event list contract

- Product/version: MaxPatrol SIEM 8.1
- Operation: `POST /api/events/v2/events`
- Source: https://help.ptsecurity.com/ru-RU/projects/siem/8.1/help/10836123659
- Retrieved: 2026-08-08
- Sample origin: request and success response published by the vendor
- Anonymization: user name, host names, message text, site metadata, and one duplicate asset ID were replaced or removed; field names and envelope structure were preserved
- Error fixture: the operation documents HTTP 400 and 401 but does not publish an error body; `error.json` is deliberately `null` instead of an invented envelope

Entity lookup is not claimed for this provider until a documented operation or a reviewed event-search predicate is available.
