package config

import (
	"strings"
	"testing"
)

func TestSecretFromEnv(t *testing.T) {
	t.Setenv("SB0RKA_IR_GATEWAY_CLIENT_SECRET", strings.Repeat("b", 32))
	secret, err := secretFromEnv("SB0RKA_IR_GATEWAY_CLIENT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != strings.Repeat("b", 32) {
		t.Fatalf("secret = %q", secret)
	}

	t.Setenv("SB0RKA_IR_GATEWAY_CLIENT_SECRET", "")
	if _, err := secretFromEnv("SB0RKA_IR_GATEWAY_CLIENT_SECRET"); err == nil {
		t.Fatal("expected empty secret to be rejected")
	}
}
