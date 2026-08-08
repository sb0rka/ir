package config

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

const (
	DefaultRequestTimeout = 15 * time.Second
	DefaultSourceTimeout  = 10 * time.Second
	DefaultMockEvents     = 50_000
	DefaultMockEndpoints  = 5_000
	DefaultMockHistory    = 90
	MaxMockEvents         = 1_000_000
	MaxMockEndpoints      = 100_000
	MaxMockHistory        = 3_650
)

var SourceCodes = []string{"maxpatrol-siem", "pt-sandbox"}

type Config struct {
	Server         ServerConfig
	Auth           AuthConfig
	Log            coreconfig.LoggerConfig
	Mock           MockConfig
	Sources        map[string]SourceConfig
	ProjectSources map[string]map[string]bool
}

type MockConfig struct {
	EventCount    int
	EndpointCount int
	HistoryDays   int
}

type ServerConfig struct {
	Addr           string
	Port           string
	CORSWhitelist  map[string]bool
	RequestTimeout time.Duration
	SourceTimeout  time.Duration
}

type AuthConfig struct {
	Disabled  bool
	PublicKey ed25519.PublicKey
	Issuer    string
	Audience  string
	Kid       string
	Typ       string
}

type SourceConfig struct {
	Mode           string
	BaseURL        string
	CredentialFile string
	Timeout        time.Duration
	TLSCAFile      string
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
			Disabled: coreconfig.GetBoolEnv("AUTH_DISABLED", false),
			Issuer:   coreconfig.GetStringEnv("ACCESS_TOKEN_ISSUER", ""),
			Audience: coreconfig.GetStringEnv("ACCESS_TOKEN_AUDIENCE", "api.local"),
			Kid:      coreconfig.GetStringEnv("ACCESS_TOKEN_KID", ""),
			Typ:      coreconfig.GetStringEnv("ACCESS_TOKEN_TYP", "access+jwt"),
		},
		Log: coreconfig.LoggerConfig{
			Level:  coreconfig.GetStringEnv("LOG_LEVEL", "info"),
			Format: coreconfig.GetStringEnv("LOG_FORMAT", "json"),
		},
		Mock: MockConfig{
			EventCount:    coreconfig.GetIntEnv("MOCK_EVENT_COUNT", DefaultMockEvents),
			EndpointCount: coreconfig.GetIntEnv("MOCK_ENDPOINT_COUNT", DefaultMockEndpoints),
			HistoryDays:   coreconfig.GetIntEnv("MOCK_HISTORY_DAYS", DefaultMockHistory),
		},
		Sources: make(map[string]SourceConfig, len(SourceCodes)),
	}
	projectSources, err := parseProjectSources(coreconfig.GetStringEnv("PROJECT_SOURCE_ALLOWLISTS", ""))
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
	if cfg.Mock.EventCount < 1 || cfg.Mock.EventCount > MaxMockEvents {
		return Config{}, fmt.Errorf("MOCK_EVENT_COUNT must be between 1 and %d", MaxMockEvents)
	}
	if cfg.Mock.EndpointCount < 1 || cfg.Mock.EndpointCount > MaxMockEndpoints {
		return Config{}, fmt.Errorf("MOCK_ENDPOINT_COUNT must be between 1 and %d", MaxMockEndpoints)
	}
	if cfg.Mock.HistoryDays < 1 || cfg.Mock.HistoryDays > MaxMockHistory {
		return Config{}, fmt.Errorf("MOCK_HISTORY_DAYS must be between 1 and %d", MaxMockHistory)
	}

	key, err := loadPublicKey()
	if err != nil {
		return Config{}, err
	}
	cfg.Auth.PublicKey = key
	if !cfg.Auth.Disabled && len(key) == 0 {
		return Config{}, fmt.Errorf("access token public key is required when auth is enabled")
	}

	for _, code := range SourceCodes {
		prefix := "SOURCE_" + strings.ToUpper(strings.ReplaceAll(code, "-", "_")) + "_"
		source := SourceConfig{
			Mode:           strings.ToLower(coreconfig.GetStringEnv(prefix+"MODE", "mock")),
			BaseURL:        coreconfig.GetStringEnv(prefix+"BASE_URL", ""),
			CredentialFile: coreconfig.GetStringEnv(prefix+"CREDENTIAL_FILE", ""),
			Timeout:        coreconfig.GetDurationEnv(prefix+"TIMEOUT_SEC", cfg.Server.SourceTimeout, time.Second),
			TLSCAFile:      coreconfig.GetStringEnv(prefix+"TLS_CA_FILE", ""),
		}
		if source.Mode != "mock" && source.Mode != "proxy" {
			return Config{}, fmt.Errorf("source %s has invalid mode %q", code, source.Mode)
		}
		if source.Mode == "proxy" {
			return Config{}, fmt.Errorf("proxy mode is not implemented for source %s", code)
		}
		cfg.Sources[code] = source
	}

	return cfg, nil
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

func loadPublicKey() (ed25519.PublicKey, error) {
	raw := []byte(coreconfig.GetStringEnv("ACCESS_TOKEN_PUBLIC_KEY", ""))
	if path := coreconfig.GetStringEnv("ACCESS_TOKEN_PUBLIC_KEY_FILE_PATH", ""); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read public key: %w", err)
		}
		raw = data
	}
	if len(raw) == 0 {
		return nil, nil
	}

	block, _ := pem.Decode(raw)
	if block != nil {
		raw = block.Bytes
	} else {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, fmt.Errorf("public key is neither PEM nor base64: %w", err)
		}
		raw = decoded
	}
	parsed, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ed25519")
	}
	return key, nil
}
