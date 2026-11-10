package wazuh

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
)

// Provider cursors travel only inside the service's authenticated event cursor.
type providerCursor struct {
	Version int    `json:"v"`
	PIT     string `json:"pit"`
	Sort    []any  `json:"sort,omitempty"`
	Total   *int64 `json:"total,omitempty"`
}

func encodeProviderCursor(cursor providerCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func decodeProviderCursor(raw string) (providerCursor, error) {
	var cursor providerCursor
	if len(raw) > 256<<10 {
		return cursor, sourceRequestError("invalid_cursor", "cursor is invalid")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursor, sourceRequestError("invalid_cursor", "cursor is invalid")
	}
	d := json.NewDecoder(bytes.NewReader(decoded))
	d.UseNumber()
	d.DisallowUnknownFields()
	if d.Decode(&cursor) != nil || d.Decode(new(any)) != io.EOF || cursor.Version != 1 || cursor.PIT == "" || len(cursor.PIT) > 128<<10 || len(cursor.Sort) > 12 {
		return providerCursor{}, sourceRequestError("invalid_cursor", "cursor is invalid")
	}
	for _, v := range cursor.Sort {
		switch v.(type) {
		case nil, string, json.Number, bool:
		default:
			return providerCursor{}, sourceRequestError("invalid_cursor", "cursor is invalid")
		}
	}
	return cursor, nil
}
