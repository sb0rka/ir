package config

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

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
	Disabled  bool
	PublicKey ed25519.PublicKey
	Issuer    string
	Audience  string
	Kid       string
	Typ       string
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
	key, err := loadPublicKey()
	if err != nil {
		return Config{}, err
	}
	cfg.Auth.PublicKey = key
	if !cfg.Auth.Disabled && len(key) == 0 {
		return Config{}, fmt.Errorf("access token public key is required when auth is enabled")
	}
	cfg.Sources[PTMaxPatrolSIEM] = SourceConfig{
		BaseURL:          coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_BASE_URL", ""),
		IncidentsBaseURL: coreconfig.GetStringEnv("SOURCE_PT_MAXPATROL_SIEM_INCIDENTS_BASE_URL", ""),
		Timeout:          coreconfig.GetDurationEnv("SOURCE_PT_MAXPATROL_SIEM_TIMEOUT_SEC", cfg.Server.SourceTimeout, time.Second),
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
	if err != nil || value.Scheme != "https" || value.Host == "" || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return fmt.Errorf("source %s %s must be an absolute HTTPS URL without credentials, query, or fragment", source, label)
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

func loadPublicKey() (ed25519.PublicKey, error) {
	if key, err := loadPrivateKey(); err != nil || key != nil {
		return key, err
	}

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

// loadPrivateKey returns the public half of a configured Ed25519 signing key.
// Auth and API use the same private key in the demo environment, so verifier
// services can derive the public key instead of requiring a duplicate file.
//
// ACCESS_TOKEN_PRIVATE_KEY follows sb0rka Auth: base64(PKCS#8 PEM). PEM files,
// plain base64(DER), and base64(DER) files remain supported.
func loadPrivateKey() (ed25519.PublicKey, error) {
	raw := []byte(coreconfig.GetStringEnv("ACCESS_TOKEN_PRIVATE_KEY", ""))
	if path := coreconfig.GetStringEnv("ACCESS_TOKEN_PRIVATE_KEY_FILE_PATH", ""); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		raw = data
	}
	if len(raw) == 0 {
		return nil, nil
	}

	der, err := decodePrivateKeyDER(raw)
	if err != nil {
		return nil, err
	}

	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ed25519")
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("cannot derive public key")
	}
	return public, nil
}

func decodePrivateKeyDER(raw []byte) ([]byte, error) {
	block, _ := pem.Decode(raw)
	if block != nil {
		return block.Bytes, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("private key is neither PEM nor base64: %w", err)
	}

	block, _ = pem.Decode(decoded)
	if block != nil {
		return block.Bytes, nil
	}

	return decoded, nil
}
