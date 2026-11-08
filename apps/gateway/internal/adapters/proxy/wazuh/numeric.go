package wazuh

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"
)

// Wazuh decoders emit optional CVSS scores as either JSON numbers or strings.
type wireFloat float64

func (value *wireFloat) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, []byte("null")) {
		*value = 0
		return nil
	}
	text := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
	}
	if text == "" {
		*value = 0
		return nil
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return errors.New("invalid numeric source field")
	}
	*value = wireFloat(number)
	return nil
}
