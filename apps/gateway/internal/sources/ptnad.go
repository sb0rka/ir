package sources

import (
	"errors"
	"fmt"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/ptnad"
	gatewayproxy "github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

const (
	PTNAD       = ptnad.SourceCode
	PTNADCookie = ptnad.CredentialSecretName
)

func ptnadDefinition() Definition {
	return Definition{
		Code: PTNAD, CredentialSelector: PTNADCookie,
		Load: loadPTNAD, DecodeCredentials: func(fields map[string]string) (capability.Access, error) {
			cookie, err := ptnadCookie(fields)
			return capability.Access{Cookie: cookie}, err
		},
	}
}

func loadPTNAD(defaultTimeout time.Duration) (ConfiguredSource, bool, error) {
	baseURL := coreconfig.GetStringEnv("SOURCE_PT_NAD_BASE_URL", "")
	storeIDs, err := parseStoreIDs(coreconfig.GetStringEnv("SOURCE_PT_NAD_STORE_IDS", ""))
	if err != nil {
		return ConfiguredSource{}, false, err
	}
	if baseURL == "" && len(storeIDs) == 0 {
		return ConfiguredSource{}, false, nil
	}
	settings := gatewayproxy.HTTPClientConfig{
		BaseURL: baseURL, Timeout: coreconfig.GetDurationEnv("SOURCE_PT_NAD_TIMEOUT_SEC", defaultTimeout, time.Second),
		TLSCAFile: coreconfig.GetStringEnv("SOURCE_PT_NAD_TLS_CA_FILE", ""),
	}
	if settings.Timeout <= 0 {
		return ConfiguredSource{}, false, fmt.Errorf("source %s timeout must be positive", PTNAD)
	}
	if err := validateSourceURL(PTNAD, "base URL", baseURL); err != nil {
		return ConfiguredSource{}, false, err
	}
	if len(storeIDs) == 0 {
		return ConfiguredSource{}, false, fmt.Errorf("source %s requires SOURCE_PT_NAD_STORE_IDS", PTNAD)
	}
	instances := make([]string, 0, len(storeIDs))
	for _, id := range storeIDs {
		instances = append(instances, strconv.FormatInt(id, 10))
	}
	return ConfiguredSource{Code: PTNAD, InstanceIDs: instances, Fingerprint: settingsFingerprint(settings, storeIDs), Build: func(options BuildOptions) (registry.Provider, error) {
		config := settings
		config.SkipTLSVerify = options.SkipTLSVerify
		transport, err := gatewayproxy.NewHTTPClient(config)
		if err != nil {
			return registry.Provider{}, err
		}
		client, err := ptnad.NewClient(ptnad.Config{BaseURL: baseURL, HTTPClient: transport.Client})
		if err != nil {
			return registry.Provider{}, err
		}
		provider, err := ptnad.NewProvider(client, storeIDs)
		if err != nil {
			return registry.Provider{}, err
		}
		return provider.RegistryProvider(), nil
	}}, true, nil
}

func parseStoreIDs(raw string) ([]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[int]bool)
	values := []int64{}
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 || seen[value] {
			return nil, fmt.Errorf("SOURCE_PT_NAD_STORE_IDS must contain unique positive integers")
		}
		seen[value] = true
		values = append(values, int64(value))
	}
	slices.Sort(values)
	return values, nil
}

func ptnadCookie(fields map[string]string) (string, error) {
	cookie, err := cookieHeader(fields, "sessionid", "csrftoken")
	if err != nil {
		return "", err
	}
	_, session := fields["sessionid"]
	_, csrf := fields["csrftoken"]
	if !session || !csrf {
		return "", errors.New("NAD cookie configuration incomplete")
	}
	return cookie, nil
}
