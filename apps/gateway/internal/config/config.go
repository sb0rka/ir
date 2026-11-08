package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/sources"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

const (
	DefaultRequestTimeout = 15 * time.Second
	DefaultSourceTimeout  = 10 * time.Second
	PTMaxPatrolSIEM       = sources.PTMaxPatrolSIEM
	PTNAD                 = sources.PTNAD
)

type Config struct {
	CursorSigningKey        []byte
	Server                  ServerConfig
	Auth                    AuthConfig
	Log                     coreconfig.LoggerConfig
	Sb0rkaAPIBaseURL        string
	SkipTLSVerify           bool
	Sources                 map[string]sources.ConfiguredSource
	Database                coreconfig.DatabaseConfig
	GatewayConfigurationKey []byte
}

type ServerConfig struct {
	Addr           string
	Port           string
	CORSWhitelist  map[string]bool
	RequestTimeout time.Duration
	SourceTimeout  time.Duration
}

type AuthConfig struct {
	APIBaseURL        string
	Issuer            string
	ClientID          string
	ClientSecret      []byte
	Keys              *coreauth.JWKSCache
	AccessTokenPolicy *coreauth.AccessTokenPolicy
}

func Load(catalog *sources.Catalog) (Config, error) {
	if catalog == nil {
		return Config{}, fmt.Errorf("source catalog is required")
	}
	cfg := Config{
		Server: ServerConfig{
			Addr:           coreconfig.GetStringEnv("SERVER_ADDR", "0.0.0.0"),
			Port:           coreconfig.GetStringEnv("SERVER_PORT", "8091"),
			CORSWhitelist:  coreconfig.ParseCORSWhitelist(coreconfig.GetStringEnv("SERVER_CORS_WHITELIST", "")),
			RequestTimeout: coreconfig.GetDurationEnv("REQUEST_TIMEOUT_SEC", DefaultRequestTimeout, time.Second),
			SourceTimeout:  coreconfig.GetDurationEnv("SOURCE_TIMEOUT_SEC", DefaultSourceTimeout, time.Second),
		},
		Auth: AuthConfig{
			Issuer:   strings.TrimRight(coreconfig.GetStringEnv("SB0RKA_AUTH_BASE_URL", ""), "/"),
			ClientID: coreconfig.GetStringEnv("AUTH_INTROSPECTION_CLIENT_ID", coreauth.ClientIRGateway),
		},
		Log: coreconfig.LoggerConfig{
			Level:  coreconfig.GetStringEnv("LOG_LEVEL", "info"),
			Format: coreconfig.GetStringEnv("LOG_FORMAT", "json"),
		},
		Sb0rkaAPIBaseURL: coreconfig.GetStringEnv("SB0RKA_API_BASE_URL", ""),
		SkipTLSVerify:    coreconfig.GetBoolEnv("GATEWAY_SKIP_TLS_VERIFY", false),
		Database: coreconfig.DatabaseConfig{
			URI:             coreconfig.GetStringEnv("DATABASE_URI", ""),
			MaxConns:        coreconfig.GetIntEnv("DATABASE_MAX_CONNS", coreconfig.DefaultDatabaseMaxConns),
			ConnMaxLifetime: coreconfig.GetDurationEnv("DATABASE_CONN_MAX_LIFETIME_SEC", coreconfig.DefaultDatabaseConnMaxLifetime, time.Second),
		},
	}
	var err error
	cfg.GatewayConfigurationKey, err = gatewayConfigurationKey(cfg.Database.URI, os.Getenv("IR_GATEWAY_CONFIGURATION_KEY"))
	if err != nil {
		return Config{}, err
	}
	cfg.Auth.AccessTokenPolicy, err = coreauth.AccessTokenPolicyFromEnv()
	if err != nil {
		return Config{}, err
	}

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
		return Config{}, fmt.Errorf("SB0RKA_AUTH_BASE_URL: %w", err)
	}
	cfg.Auth.Keys, err = coreauth.NewJWKSCache(cfg.Auth.Issuer)
	if err != nil {
		return Config{}, err
	}
	cfg.Auth.ClientSecret, err = secretFromEnv("SB0RKA_IR_GATEWAY_CLIENT_SECRET")
	if err != nil {
		return Config{}, err
	}
	if cfg.Auth.ClientID == "" {
		return Config{}, fmt.Errorf("invalid Gateway introspection credential")
	}
	if err := coreauth.ValidateServiceOrigin(cfg.Sb0rkaAPIBaseURL, allowHTTP); err != nil {
		return Config{}, fmt.Errorf("SB0RKA_API_BASE_URL: %w", err)
	}
	cfg.Auth.APIBaseURL = cfg.Sb0rkaAPIBaseURL
	cfg.Sources, err = catalog.Load(cfg.Server.SourceTimeout)
	if err != nil {
		return Config{}, err
	}
	rawCursorKey := strings.TrimSpace(os.Getenv("GATEWAY_CURSOR_SIGNING_KEY"))
	if rawCursorKey != "" {
		cfg.CursorSigningKey, err = base64.StdEncoding.DecodeString(rawCursorKey)
		if err != nil || len(cfg.CursorSigningKey) != 32 {
			return Config{}, fmt.Errorf("GATEWAY_CURSOR_SIGNING_KEY must be a base64-encoded 32-byte key")
		}
	}

	return cfg, nil
}

func gatewayConfigurationKey(databaseURI, raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if strings.TrimSpace(databaseURI) == "" && raw == "" {
		return nil, nil
	}
	if strings.TrimSpace(databaseURI) == "" || raw == "" {
		return nil, fmt.Errorf("DATABASE_URI and IR_GATEWAY_CONFIGURATION_KEY must be configured together")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("IR_GATEWAY_CONFIGURATION_KEY must be a base64-encoded 32-byte key")
	}
	return key, nil
}

func secretFromEnv(name string) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	if len(raw) < 32 {
		return nil, fmt.Errorf("%s requires at least 32 bytes", name)
	}
	return []byte(raw), nil
}
