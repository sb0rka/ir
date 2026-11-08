package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGatewayConfigurationKey(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	for _, test := range []struct {
		name, uri, key string
		valid          bool
	}{
		{"legacy configuration", "", "", true},
		{"agent configuration", "postgres://localhost/ir", key, true},
		{"missing key", "postgres://localhost/ir", "", false},
		{"missing database", "", key, false},
		{"invalid encoding", "postgres://localhost/ir", "not-base64", false},
		{"wrong key length", "postgres://localhost/ir", base64.StdEncoding.EncodeToString([]byte("short")), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := gatewayConfigurationKey(test.uri, test.key)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t: %v", test.valid, err)
			}
			if test.valid && test.uri != "" && len(decoded) != 32 {
				t.Fatal("invalid decoded key length")
			}
		})
	}
}
