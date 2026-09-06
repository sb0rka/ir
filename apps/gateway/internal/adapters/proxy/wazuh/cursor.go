package wazuh

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type providerCursor struct {
	Version     int    `json:"v"`
	Sort        []any  `json:"sort"`
	Fingerprint string `json:"fp"`
}

func encodeCursor(cursor providerCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", &RequestError{Operation: "event search", Message: "cursor could not be encoded"}
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeCursor(raw, fingerprint string) (providerCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return providerCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return providerCursor{}, &RequestError{Operation: "event search", Message: "cursor is invalid"}
	}
	var cursor providerCursor
	if json.Unmarshal(decoded, &cursor) != nil || cursor.Version != 1 || len(cursor.Sort) == 0 {
		return providerCursor{}, &RequestError{Operation: "event search", Message: "cursor is invalid"}
	}
	if cursor.Fingerprint != fingerprint {
		return providerCursor{}, &RequestError{Operation: "event search", Message: "cursor does not match the request"}
	}
	return cursor, nil
}

func cursorFingerprint(pattern string, timeFrom, timeTo string, entityKeys []string, filter string, sortKeys []string, groupBy []string, groupValues []*string) string {
	parts := []string{
		pattern,
		timeFrom,
		timeTo,
		strings.Join(entityKeys, ","),
		strings.TrimSpace(filter),
		strings.Join(sortKeys, ","),
		strings.Join(groupBy, ","),
	}
	for _, value := range groupValues {
		if value == nil {
			parts = append(parts, "<null>")
			continue
		}
		parts = append(parts, *value)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
