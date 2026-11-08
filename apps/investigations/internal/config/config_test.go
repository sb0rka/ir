package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSecretFromEnv(t *testing.T) {
	t.Setenv("SB0RKA_IR_WEB_CLIENT_SECRET", strings.Repeat("a", 32))
	secret, err := secretFromEnv("SB0RKA_IR_WEB_CLIENT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != strings.Repeat("a", 32) {
		t.Fatalf("secret = %q", secret)
	}

	t.Setenv("SB0RKA_IR_WEB_CLIENT_SECRET", "short")
	if _, err := secretFromEnv("SB0RKA_IR_WEB_CLIENT_SECRET"); err == nil {
		t.Fatal("expected short secret to be rejected")
	}
}

func TestKeyFromEnv(t *testing.T) {
	raw := strings.Repeat("k", 32)
	t.Setenv("IR_OIDC_CONTEXT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(raw)))
	key, err := keyFromEnv("IR_OIDC_CONTEXT_ENCRYPTION_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != raw {
		t.Fatalf("key = %q", key)
	}

	t.Setenv("IR_OIDC_CONTEXT_ENCRYPTION_KEY", "not-base64")
	if _, err := keyFromEnv("IR_OIDC_CONTEXT_ENCRYPTION_KEY"); err == nil {
		t.Fatal("expected invalid key to be rejected")
	}
}
