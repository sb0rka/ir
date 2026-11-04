package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

const (
	DefaultRequestTimeout = 15 * time.Second
	DefaultSourceTimeout  = 10 * time.Second
	PTMaxPatrolSIEM       = "pt-maxpatrol-siem"
	PTNAD                 = "pt-nad"
	PTMaxPatrolCookie     = "DEMO_PT_SIEM_COOKIE"
	PTNADCookie           = "DEMO_PT_NAD_COOKIE"
)

var SourceCodes = []string{PTMaxPatrolSIEM, PTNAD}

type Config struct {
	Server           ServerConfig
	Auth             AuthConfig
	Log              coreconfig.LoggerConfig
	Sb0rkaAPIBaseURL string
	SkipTLSVerify    bool
	Sources          map[string]SourceConfig
	ProjectSources   map[string]map[string]bool
}

type ServerConfig struct {
	Addr           string
	Port           string
	CORSWhitelist  map[string]bool
	RequestTimeout time.Duration
	SourceTimeout  time.Duration
}

type AuthConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret []byte
	Keys         *coreauth.JWKSCache
}

type SourceConfig struct {
	BaseURL          string
	IncidentsBaseURL string
	StoreIDs         []string
	Timeout          time.Duration
	TLSCAFile        string
	CredentialSecret string
}

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Addr:           coreconfig.GetStringEnv("SERVER_ADDR", "0.0.0.0"),
			Port:           coreconfig.GetStringEnv("SERVER_PORT", "8091"),
			CORSWhitelist:  coreconfig.ParseCORSWhitelist(coreconfig.GetStringEnv("SERVER_CORS_WHITELIST", "")),
			RequestTimeout: coreconfig.GetDurationEnv("REQUEST_TIMEOUT_SEC", DefaultRequestTimeout, time.Second),
			SourceTimeout:  coreconfig.GetDurationEnv("SOURCE_TIMEOUT_SEC", DefaultSourceTimeout, time.Second),
		},
		Auth: AuthConfig{
			Issuer:   strings.TrimRight(coreconfig.GetStringEnv("AUTH_ISSUER", ""), "/"),
			ClientID: coreconfig.GetStringEnv("AUTH_INTROSPECTION_CLIENT_ID", coreauth.ClientIRGateway),
		},
		Log: coreconfig.LoggerConfig{
			Level:  coreconfig.GetStringEnv("LOG_LEVEL", "info"),
			Format: coreconfig.GetStringEnv("LOG_FORMAT", "json"),
		},
		Sb0rkaAPIBaseURL: coreconfig.GetStringEnv("SB0RKA_API_BASE_URL", ""),
		SkipTLSVerify:    coreconfig.GetBoolEnv("GATEWAY_SKIP_TLS_VERIFY", false),
		Sources:          make(map[string]SourceConfig, len(SourceCodes)),
	}
	projectSources, err := parseProjectSources(coreconfig.GetStringEnv("PROJECT_SOURCE_ALLOWLISTS", "{}"))
	if err != nil {
		return Config{}, err
	}
	cfg.ProjectSources = projectSources

	if cfg.Server.RequestTimeout <= 0 || cfg.Server.SourceTimeout <= 0 {
		return Config{}, fmt.Errorf("request and source timeouts must be positive")
	}
	if cfg.Server.SourceTimeout > cfg.Server.RequestTimeout {
		return Config{}, fmt.Errorf("source timeout must not exceed request timeout")
	}
	allowHTTP, err := coreconfig.GetBoolEnvStrict("AUTH_ALLOW_INSECURE_HTTP_ISSUER", false)
	if err != nil {
		return Config{}, err
	}
	if err := coreauth.ValidateServiceOrigin(cfg.Auth.Issuer, allowHTTP); err != nil {
		return Config{}, fmt.Errorf("AUTH_ISSUER: %w", err)
	}
	cfg.Auth.Keys, err = coreauth.NewJWKSCache(cfg.Auth.Issuer)
	if err != nil {
		return Config{}, err
	}
	secretPath := coreconfig.GetStringEnv("AUTH_INTROSPECTION_CLIENT_SECRET_FILE_PATH", "")
	secret, err := os.ReadFile(secretPath)
	if err != nil {
		return Config{}, fmt.Errorf("read Gateway introspection credential: %w", err)
	}
	cfg.Auth.ClientSecret = bytes.TrimSuffix(secret, []byte("\n"))
	if len(cfg.Auth.ClientSecret) < 32 || cfg.Auth.ClientID == "" {
		return Config{}, fmt.Errorf("invalid Gateway introspection credential")
	}
	// Source credentials are intentionally not wired during the IR auth migration.
	cfg.Sb0rkaAPIBaseURL = ""
	cfg.Sources[PTMaxPatrolSIEM] = SourceConfig{
		BaseURL:          coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_BASE_URL", ""),
		IncidentsBaseURL: coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL", ""),
		Timeout: coreconfig.GetDurationEnv(
			"SOURCE_PT_MAXPATROL_SIEM_TIMEOUT_SEC",
			cfg.Server.SourceTimeout,
			time.Second,
		),
		TLSCAFile:        coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_TLS_CA_FILE", ""),
		CredentialSecret: PTMaxPatrolCookie,
	}
	storeIDs, storeErr := parseStoreIDs(coreconfig.GetStringEnv("SOURCE_PT_NAD_STORE_IDS", ""))
	if storeErr != nil {
		return Config{}, storeErr
	}
	cfg.Sources[PTNAD] = SourceConfig{
		BaseURL:          coreconfig.GetStringEnv("SOURCE_PT_NAD_BASE_URL", ""),
		StoreIDs:         storeIDs,
		Timeout:          coreconfig.GetDurationEnv("SOURCE_PT_NAD_TIMEOUT_SEC", cfg.Server.SourceTimeout, time.Second),
		TLSCAFile:        coreconfig.GetStringEnv("SOURCE_PT_NAD_TLS_CA_FILE", ""),
		CredentialSecret: PTNADCookie,
	}

	for code := range configuredSources(cfg.ProjectSources) {
		source := cfg.Sources[code]
		if source.Timeout <= 0 {
			return Config{}, fmt.Errorf("source %s timeout must be positive", code)
		}
		if err := validateSourceURL(code, "base URL", source.BaseURL); err != nil {
			return Config{}, err
		}
		switch code {
		case PTMaxPatrolSIEM:
			if err := validateSourceURL(code, "incidents base URL", source.IncidentsBaseURL); err != nil {
				return Config{}, err
			}
		case PTNAD:
			if len(source.StoreIDs) == 0 {
				return Config{}, fmt.Errorf("source %s requires SOURCE_PT_NAD_STORE_IDS", code)
			}
		}
	}

	return cfg, nil
}

func configuredSources(projects map[string]map[string]bool) map[string]struct{} {
	result := make(map[string]struct{})
	for _, sources := range projects {
		for source := range sources {
			result[source] = struct{}{}
		}
	}
	return result
}

func validateSourceURL(source, label, raw string) error {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Scheme != "https" || value.Host == "" || value.User != nil || value.RawQuery != "" ||
		value.Fragment != "" {
		return fmt.Errorf(
			"source %s %s must be an absolute HTTPS URL without credentials, query, or fragment",
			source,
			label,
		)
	}
	return nil
}

func parseStoreIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[int]struct{})
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("SOURCE_PT_NAD_STORE_IDS must contain unique positive integers")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("SOURCE_PT_NAD_STORE_IDS must contain unique positive integers")
		}
		seen[value] = struct{}{}
	}
	values := make([]int, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Ints(values)
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.Itoa(value))
	}
	return result, nil
}

func parseProjectSources(raw string) (map[string]map[string]bool, error) {
	result := map[string]map[string]bool{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	var values map[string][]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("parse PROJECT_SOURCE_ALLOWLISTS: %w", err)
	}
	known := make(map[string]bool, len(SourceCodes))
	for _, code := range SourceCodes {
		known[code] = true
	}
	for projectID, sources := range values {
		if !validProjectID(projectID) {
			return nil, fmt.Errorf("PROJECT_SOURCE_ALLOWLISTS contains invalid project %q", projectID)
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("PROJECT_SOURCE_ALLOWLISTS project %q has no sources", projectID)
		}
		allowed := make(map[string]bool, len(sources))
		for _, source := range sources {
			source = strings.TrimSpace(source)
			if !known[source] {
				return nil, fmt.Errorf("PROJECT_SOURCE_ALLOWLISTS contains unknown source %q", source)
			}
			allowed[source] = true
		}
		result[projectID] = allowed
	}
	return result, nil
}

func validProjectID(value string) bool {
	if len(value) < 10 || len(value) > 12 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
