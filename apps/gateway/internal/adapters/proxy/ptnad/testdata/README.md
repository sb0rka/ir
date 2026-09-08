# NAD case fixtures

Selected response fields from sb0rka/docs-internal PR #6, head
`6f5747dea2b98865feafd28127f4dd1eff27c040`:

- `shell-search.json`: selected shell row from NAD_2.1/queries/01-nad-alerts-list-2023-06-08-store-19.json
- `shell-alert.json`: NAD_2.1/queries/04-nad-open-alert-vRwB4Z4BaLX3hldUvkZ1-CMD-EXE-shell-banner.json
- `imap-session.json`: NAD_2.6/queries/07-nad-open-session-P8H9AvNOmgVjbVc4HAvvX0-imap-srochnoe-obnovlenie.json
- `file-session.json`: NAD_2.6/queries/04-nad-open-session-P8H9F5zUMIAFzrmsj6-mk1-chromeupdate-exe.json

No HTTP credentials, mailbox passwords, executable files or vendor download URLs.
The shell banner retains the original Base64 bytes. Export task responses in tests
are synthetic instances of the captured sources/getfile -> tasks -> download contract.
These fixtures do not establish live NAD availability or PCAP export support.
