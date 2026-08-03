# Provider mappings

The mock providers describe one attack scenario. Their IDs and timestamps are deterministic. The source fixture is parsed into canonical records; UI-only `position`, `style`, and `animated` fields are not part of the scenario model or responses.

## MaxPatrol SIEM (`maxpatrol-siem`)

Capabilities: event search and entity lookup. Raw and correlated records become canonical events; correlation metadata stays in bounded attributes. Reference: [MaxPatrol SIEM event list API](https://help.ptsecurity.com/ru-RU/projects/siem/8.1/help/10836123659).

## PT NAD (`pt-nad`)

Capabilities: event search and entity lookup. Network sessions and attacks map protocol, endpoints, byte counters, HTTP metadata, alert fields, and extracted-file presence. `NAD.json` was used only as a field dictionary, not copied as a response fixture. Reference: [PT NAD REST API](https://help.ptsecurity.com/en-US/projects/nad/12.0/help/4750761739).

## MaxPatrol EDR (`maxpatrol-edr`)

Capabilities: endpoint search and response-action catalogue. The catalogue is read-only; Gateway has no action-execution route. Event search is intentionally absent because no suitable documented public contract was established. Reference: [MaxPatrol EDR public API](https://help.ptsecurity.com/ru-RU/projects/edr/9.1/help/7404729995).

## PT Sandbox (`pt-sandbox`)

Capability: artifact analysis. Known documents, scripts, executables, and extracted artifacts map to deterministic analyses, engine/behavior attributes, hashes, and verdicts. References: [upload](https://help.ptsecurity.com/ru-RU/projects/sb/5.24/developer/1237958027) and [scan](https://help.ptsecurity.com/ru-RU/projects/sb/5.18/developer/5040429451).

## PT Fusion (`pt-fusion`)

Capability: entity lookup. IP, domain, and hash reputation map to verdict, confidence, labels, related indicators, and canonical relations. Reference: [PT Fusion search](https://fusion.ptsecurity.com/docs/portal/threat-research/search/).
