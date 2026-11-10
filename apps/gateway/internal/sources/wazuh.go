package sources

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/wazuh"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	gatewayproxy "github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

func wazuhDefinition() Definition {
	return Definition{Code: wazuh.SourceCode, CredentialSelector: wazuh.CredentialSelector, Load: loadWazuh, DecodeCredentials: wazuhCredentials}
}
func loadWazuh(defaultTimeout time.Duration) (ConfiguredSource, bool, error) {
	baseURL := coreconfig.GetStringEnv("SOURCE_WAZUH_BASE_URL", "")
	if baseURL == "" {
		return ConfiguredSource{}, false, nil
	}
	if err := validateSourceURL(wazuh.SourceCode, "base URL", baseURL); err != nil {
		return ConfiguredSource{}, false, err
	}
	timeout, err := coreconfig.GetDurationEnvStrict("SOURCE_WAZUH_TIMEOUT_SEC", defaultTimeout, time.Second)
	if err != nil {
		return ConfiguredSource{}, false, err
	}
	if timeout <= 0 {
		return ConfiguredSource{}, false, errors.New("Wazuh timeout must be positive")
	}
	skip, err := coreconfig.GetBoolEnvStrict("SOURCE_WAZUH_SKIP_TLS_VERIFY", false)
	if err != nil {
		return ConfiguredSource{}, false, err
	}
	settings := wazuh.ClientConfig{HTTP: gatewayproxy.HTTPClientConfig{BaseURL: baseURL, Timeout: timeout, TLSCAFile: coreconfig.GetStringEnv("SOURCE_WAZUH_TLS_CA_FILE", ""), SkipTLSVerify: skip}, IndexPattern: coreconfig.GetStringEnv("SOURCE_WAZUH_INDEX_PATTERN", "wazuh-alerts-*"), DocumentIDField: coreconfig.GetStringEnv("SOURCE_WAZUH_DOCUMENT_ID_FIELD", "_id")}
	// Validate settings at startup without making a vendor call or loading credentials.
	if err := wazuh.ValidateIndexSettings(settings.IndexPattern, settings.DocumentIDField); err != nil {
		return ConfiguredSource{}, false, err
	}
	return ConfiguredSource{Code: wazuh.SourceCode, Fingerprint: settingsFingerprint(settings.HTTP, settings.IndexPattern, settings.DocumentIDField), Build: func(options BuildOptions) (registry.Provider, error) {
		cfg := settings
		cfg.HTTP.SkipTLSVerify = cfg.HTTP.SkipTLSVerify || options.SkipTLSVerify
		return wazuh.NewProvider(cfg)
	}}, true, nil
}
func wazuhCredentials(fields map[string]string) (capability.Access, error) {
	username, userOK := fields["username"]
	password, passOK := fields["password"]
	if !userOK || !passOK || username == "" || password == "" || strings.Contains(username, ":") || strings.IndexFunc(username, unicode.IsControl) >= 0 || strings.IndexFunc(password, unicode.IsControl) >= 0 {
		return capability.Access{}, errors.New("Wazuh credentials are incomplete or invalid")
	}
	return capability.Access{Basic: &capability.BasicCredentials{Username: username, Password: password}}, nil
}
