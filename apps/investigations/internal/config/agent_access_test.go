package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAgentConfigurationEncryptionKey(t *testing.T) {
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	for name, value := range map[string]string{
		"SB0RKA_AUTH_BASE_URL":            "http://localhost:8020",
		"SB0RKA_API_BASE_URL":             "http://localhost:8080",
		"GATEWAY_BASE_URL":                "http://localhost:8091",
		"IR_PUBLIC_BASE_URL":              "http://localhost:8090",
		"IR_WEB_URL":                      "http://localhost:3400",
		"SERVER_CORS_WHITELIST":           "http://localhost:3400",
		"DATABASE_URI":                    "postgres://localhost/investigations",
		"AUTH_ALLOW_INSECURE_HTTP_ISSUER": "true",
		"SB0RKA_IR_WEB_CLIENT_SECRET":     strings.Repeat("w", 32),
		"IR_SESSION_ENCRYPTION_KEY":       encode(strings.Repeat("s", 32)),
		"IR_OIDC_CONTEXT_ENCRYPTION_KEY":  encode(strings.Repeat("c", 32)),
	} {
		t.Setenv(name, value)
	}
	for _, test := range []struct {
		name, key string
		valid     bool
	}{
		{"legacy setup", "", true},
		{"independent key", encode(strings.Repeat("g", 32)), true},
		{"invalid encoding", "not-base64", false},
		{"wrong length", encode("short"), false},
		{"session key reuse", encode(strings.Repeat("s", 32)), false},
		{"context key reuse", encode(strings.Repeat("c", 32)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("IR_GATEWAY_CONFIGURATION_KEY", test.key)
			cfg, err := Load()
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t, error=%v", test.valid, err)
			}
			if test.valid && test.key != "" && len(cfg.Gateway.ConfigurationKey) != 32 {
				t.Fatal("configuration key was not loaded")
			}
		})
	}
}
