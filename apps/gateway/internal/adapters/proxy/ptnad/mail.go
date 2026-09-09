package ptnad

import (
	"encoding/json"
	"strings"
)

// NAD stores the message date in MIME headers; older captures flatten it to date.
// Keep arbitrary MIME headers out of the canonical session and event responses.
func (hint *MailHint) UnmarshalJSON(data []byte) error {
	type plain MailHint
	var wire struct {
		plain
		Headers []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"headers"`
		Keys   []string          `json:"headers.key"`
		Values []json.RawMessage `json:"headers.value"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*hint = MailHint(wire.plain)
	for _, header := range wire.Headers {
		if hint.Date == "" && strings.EqualFold(header.Key, "Date") {
			_ = json.Unmarshal(header.Value, &hint.Date)
		}
	}
	for index, key := range wire.Keys {
		if hint.Date == "" && strings.EqualFold(key, "Date") && index < len(wire.Values) {
			_ = json.Unmarshal(wire.Values[index], &hint.Date)
		}
	}
	return nil
}
