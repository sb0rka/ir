// Package irconfig defines the document understood by IR API and Gateway.
// The storage service preserves the original JSON text.
package irconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SecretName      = "IR/SOURCE_CONFIGURATION"
	MaxBytes        = 64 << 10
	MaxRequestBytes = 512 << 10
	TokenHeader     = "X-IR-Config-Token"
)

var ErrInvalid = errors.New("configuration must be a JSON array of objects with unique nonempty names and string fields, at most 64 KiB")

type Metadata struct {
	SecretID   string     `json:"-"`
	Configured bool       `json:"configured"`
	Version    int32      `json:"version"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type Document struct {
	Metadata
	Value  string                       `json:"value"`
	Values map[string]map[string]string `json:"-"`
}

// Parse rejects duplicate decoded keys and source names, including escaped spellings.
func Parse(raw []byte) (map[string]map[string]string, error) {
	if !utf8.Valid(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, err := d.Token()
	if err != nil || t != json.Delim('[') {
		return nil, ErrInvalid
	}
	sources := map[string]map[string]string{}
	for d.More() {
		t, err = d.Token()
		if err != nil || t != json.Delim('{') {
			return nil, ErrInvalid
		}
		values := map[string]string{}
		for d.More() {
			t, err = d.Token()
			key, ok := t.(string)
			if err != nil || !ok {
				return nil, ErrInvalid
			}
			if _, exists := values[key]; exists {
				return nil, ErrInvalid
			}
			t, err = d.Token()
			value, ok := t.(string)
			if err != nil || !ok {
				return nil, ErrInvalid
			}
			values[key] = value
		}
		if t, err = d.Token(); err != nil || t != json.Delim('}') {
			return nil, ErrInvalid
		}
		name := values["name"]
		if strings.TrimSpace(name) == "" {
			return nil, ErrInvalid
		}
		if _, exists := sources[name]; exists {
			return nil, ErrInvalid
		}
		delete(values, "name")
		sources[name] = values
	}
	if t, err = d.Token(); err != nil || t != json.Delim(']') {
		return nil, ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, ErrInvalid
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil || compact.Len() > MaxBytes {
		return nil, ErrInvalid
	}
	return sources, nil
}
