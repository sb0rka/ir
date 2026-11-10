package sources

import (
	"errors"
	"fmt"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	gatewayproxy "github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

const (
	PTMaxPatrolSIEM   = maxpatrol.SourceCode
	PTMaxPatrolCookie = maxpatrol.CredentialSecretName
)

func maxpatrolDefinition() Definition {
	return Definition{
		Code: PTMaxPatrolSIEM, CredentialSelector: PTMaxPatrolCookie,
		Load: loadMaxpatrol, DecodeCredentials: func(fields map[string]string) (capability.Access, error) {
			cookie, err := maxpatrolCookie(fields)
			return capability.Access{Cookie: cookie}, err
		},
	}
}

func loadMaxpatrol(defaultTimeout time.Duration) (ConfiguredSource, bool, error) {
	baseURL := coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_BASE_URL", "")
	incidentsURL := coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL", "")
	if baseURL == "" && incidentsURL == "" {
		return ConfiguredSource{}, false, nil
	}
	timeout := coreconfig.GetDurationEnv("SOURCE_PT_MAXPATROL_SIEM_TIMEOUT_SEC", defaultTimeout, time.Second)
	if timeout <= 0 {
		return ConfiguredSource{}, false, fmt.Errorf("source %s timeout must be positive", PTMaxPatrolSIEM)
	}
	if err := validateSourceURL(PTMaxPatrolSIEM, "base URL", baseURL); err != nil {
		return ConfiguredSource{}, false, err
	}
	if err := validateSourceURL(PTMaxPatrolSIEM, "incidents base URL", incidentsURL); err != nil {
		return ConfiguredSource{}, false, err
	}
	caFile := coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_TLS_CA_FILE", "")
	settings := maxpatrol.ClientConfig{
		SIEM:      gatewayproxy.HTTPClientConfig{BaseURL: baseURL, Timeout: timeout, TLSCAFile: caFile},
		Incidents: gatewayproxy.HTTPClientConfig{BaseURL: incidentsURL, Timeout: timeout, TLSCAFile: caFile},
	}
	return ConfiguredSource{Code: PTMaxPatrolSIEM, Fingerprint: settingsFingerprint(settings.SIEM, settings.Incidents), Build: func(options BuildOptions) (registry.Provider, error) {
		config := settings
		config.SIEM.SkipTLSVerify = options.SkipTLSVerify
		config.Incidents.SkipTLSVerify = options.SkipTLSVerify
		return maxpatrol.NewProvider(config)
	}}, true, nil
}

func maxpatrolCookie(fields map[string]string) (string, error) {
	names := []string{"CorePortalCookie", "idsrv.session", "idsrv", "IncidentManagementPortalCookie"}
	cookie, err := cookieHeader(fields, names...)
	if err != nil {
		return "", err
	}
	count := 0
	for _, name := range names[:3] {
		if _, present := fields[name]; present {
			count++
		}
	}
	_, portal := fields["IncidentManagementPortalCookie"]
	if count != 0 && count != 3 || count == 0 && !portal {
		return "", errors.New("SIEM cookie configuration incomplete")
	}
	return cookie, nil
}
