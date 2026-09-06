package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadAcceptsWazuhWhenAllowlisted(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"aaaaaaaaaa":["wazuh"]}`)
	t.Setenv("SOURCE_WAZUH_BASE_URL", "https://wazuh-indexer.example:9200")
	t.Setenv("SOURCE_WAZUH_USERNAME", "admin")
	t.Setenv("SOURCE_WAZUH_PASSWORD", "secret")
	t.Setenv("SOURCE_WAZUH_INSECURE_SKIP_VERIFY", "true")
	clearOptionalWazuh(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	source := cfg.Sources[Wazuh]
	if source.BaseURL != "https://wazuh-indexer.example:9200" {
		t.Fatalf("base URL = %q", source.BaseURL)
	}
	if source.Username != "admin" || source.Password != "secret" {
		t.Fatal("credentials were not loaded")
	}
	if !source.SkipTLSVerify {
		t.Fatal("expected insecure TLS flag")
	}
	if source.IndexPattern != DefaultWazuhIndex {
		t.Fatalf("index pattern = %q", source.IndexPattern)
	}
}

func TestLoadRejectsIncompleteWazuhConfig(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"aaaaaaaaaa":["wazuh"]}`)
	t.Setenv("SOURCE_WAZUH_BASE_URL", "https://wazuh-indexer.example:9200")
	t.Setenv("SOURCE_WAZUH_USERNAME", "admin")
	t.Setenv("SOURCE_WAZUH_PASSWORD", "")
	clearOptionalWazuh(t)

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SOURCE_WAZUH_PASSWORD") {
		t.Fatalf("expected password error, got %v", err)
	}
}

func TestLoadRejectsInvalidWazuhIndexPattern(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"aaaaaaaaaa":["wazuh"]}`)
	t.Setenv("SOURCE_WAZUH_BASE_URL", "https://wazuh-indexer.example:9200")
	t.Setenv("SOURCE_WAZUH_USERNAME", "admin")
	t.Setenv("SOURCE_WAZUH_PASSWORD", "secret")
	t.Setenv("SOURCE_WAZUH_INDEX_PATTERN", "wazuh-archives-*")
	_ = os.Unsetenv("SOURCE_WAZUH_TIMEOUT_SEC")
	_ = os.Unsetenv("SOURCE_WAZUH_TLS_CA_FILE")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "index pattern") {
		t.Fatalf("expected index pattern error, got %v", err)
	}
}

func TestLoadRejectsUnknownSourceInAllowlist(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"aaaaaaaaaa":["not-a-source"]}`)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("expected unknown source error, got %v", err)
	}
}

func TestLoadPTSourcesUnaffectedByWazuhEnv(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("PROJECT_SOURCE_ALLOWLISTS", `{"aaaaaaaaaa":["pt-maxpatrol-siem","pt-nad"]}`)
	t.Setenv("SOURCE_PT_MAXPATROL_SIEM_BASE_URL", "https://siem.example")
	t.Setenv("SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL", "https://siem.example:8887")
	t.Setenv("SOURCE_PT_NAD_BASE_URL", "https://nad.example")
	t.Setenv("SOURCE_PT_NAD_STORE_IDS", "19,26")
	t.Setenv("SOURCE_WAZUH_BASE_URL", "")
	t.Setenv("SOURCE_WAZUH_USERNAME", "")
	t.Setenv("SOURCE_WAZUH_PASSWORD", "")
	clearOptionalWazuh(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sources[PTMaxPatrolSIEM].CredentialSecret != PTMaxPatrolCookie {
		t.Fatal("PT SIEM credential secret changed")
	}
	if cfg.Sources[PTNAD].CredentialSecret != PTNADCookie {
		t.Fatal("PT NAD credential secret changed")
	}
	if len(cfg.Sources[PTNAD].StoreIDs) != 2 {
		t.Fatalf("store IDs = %#v", cfg.Sources[PTNAD].StoreIDs)
	}
}

func TestValidateWazuhIndexPattern(t *testing.T) {
	valid := []string{"wazuh-alerts-*", "wazuh-alerts-4.x-*", "wazuh-alerts-4.x-2026.09.05"}
	for _, pattern := range valid {
		if err := validateWazuhIndexPattern(pattern); err != nil {
			t.Fatalf("%q: %v", pattern, err)
		}
	}
	invalid := []string{"", "alerts-*", "wazuh-alerts-*/../x", "wazuh-alerts-a,b", "wazuh-alerts-a b", "wazuh-alerts-"}
	for _, pattern := range invalid {
		if err := validateWazuhIndexPattern(pattern); err == nil {
			t.Fatalf("%q: expected error", pattern)
		}
	}
}

func clearOptionalWazuh(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SOURCE_WAZUH_TIMEOUT_SEC",
		"SOURCE_WAZUH_TLS_CA_FILE",
		"SOURCE_WAZUH_INDEX_PATTERN",
	} {
		_ = os.Unsetenv(key)
	}
}
