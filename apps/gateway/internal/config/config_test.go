package config

import (
	"strings"
	"testing"
)

func TestLoadDefaultsToMockWithExplicitDevBypass(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	for _, code := range SourceCodes {
		prefix := "SOURCE_" + strings.ToUpper(strings.ReplaceAll(code, "-", "_")) + "_"
		t.Setenv(prefix+"MODE", "mock")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 5 || !cfg.Auth.Disabled {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsUnimplementedProxyMode(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("SOURCE_PT_NAD_MODE", "proxy")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "proxy mode is not implemented for source pt-nad") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadParsesProjectSourceAllowlists(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"abcdef1234":["pt-nad","pt-fusion"]}`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProjectSources["abcdef1234"]["pt-nad"] || cfg.ProjectSources["abcdef1234"]["maxpatrol-siem"] {
		t.Fatalf("unexpected allowlist: %#v", cfg.ProjectSources)
	}
}

func TestLoadRequiresKeyWhenAuthEnabled(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "false")
	t.Setenv("ACCESS_TOKEN_PUBLIC_KEY", "")
	t.Setenv("ACCESS_TOKEN_PUBLIC_KEY_FILE_PATH", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "public key is required") {
		t.Fatalf("got %v", err)
	}
}
