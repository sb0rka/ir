package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Addr:          coreconfig.GetStringEnv("SERVER_ADDR", "0.0.0.0"),
			Port:          coreconfig.GetStringEnv("SERVER_PORT", "8090"),
			CORSWhitelist: coreconfig.ParseCORSWhitelist(coreconfig.GetStringEnv("SERVER_CORS_WHITELIST", "")),
			Auth: AuthConfig{
				Issuer:   strings.TrimRight(coreconfig.GetStringEnv("SB0RKA_AUTH_BASE_URL", ""), "/"),
				ClientID: coreconfig.GetStringEnv("OIDC_CLIENT_ID", coreauth.ClientIRWeb),
				PublicBaseURL: strings.TrimRight(
					coreconfig.GetStringEnv("IR_PUBLIC_BASE_URL", "http://localhost:8090"),
					"/",
				),
				WebURL:       strings.TrimRight(coreconfig.GetStringEnv("IR_WEB_URL", "http://localhost:5174"), "/"),
				CookieSecure: true,
			},
		},
		Database: coreconfig.DatabaseConfig{
			URI: coreconfig.GetStringEnv(
				"DATABASE_URI",
				"",
			), MaxConns: coreconfig.GetIntEnv("DATABASE_MAX_CONNS", coreconfig.DefaultDatabaseMaxConns),
			ConnMaxLifetime: coreconfig.GetDurationEnv(
				"DATABASE_CONN_MAX_LIFETIME_SEC",
				coreconfig.DefaultDatabaseConnMaxLifetime,
				time.Second,
			),
		},
		Log: coreconfig.LoggerConfig{
			Level:  coreconfig.GetStringEnv("LOG_LEVEL", "info"),
			Format: coreconfig.GetStringEnv("LOG_FORMAT", "json"),
		},
		Platform: PlatformConfig{
			APIBaseURL: strings.TrimRight(coreconfig.GetStringEnv("SB0RKA_API_BASE_URL", ""), "/"),
		},
		Gateway: GatewayConfig{BaseURL: strings.TrimRight(coreconfig.GetStringEnv("GATEWAY_BASE_URL", ""), "/")},
	}
	allowHTTP, err := coreconfig.GetBoolEnvStrict("AUTH_ALLOW_INSECURE_HTTP_ISSUER", false)
	if err != nil {
		return cfg, err
	}
	a := &cfg.Server.Auth
	a.AccessTokenPolicy, err = coreauth.AccessTokenPolicyFromEnv()
	if err != nil {
		return cfg, err
	}
	for _, origin := range []struct{ name, value string }{
		{"SB0RKA_AUTH_BASE_URL", a.Issuer},
		{"IR_PUBLIC_BASE_URL", a.PublicBaseURL},
		{"IR_WEB_URL", a.WebURL},
		{"SB0RKA_API_BASE_URL", cfg.Platform.APIBaseURL},
		{"GATEWAY_BASE_URL", cfg.Gateway.BaseURL},
	} {
		if err := coreauth.ValidateServiceOrigin(origin.value, allowHTTP); err != nil {
			return cfg, fmt.Errorf("%s: %w", origin.name, err)
		}
	}
	a.AllowHTTP = allowHTTP
	a.CookieSecure = strings.HasPrefix(a.PublicBaseURL, "https://")
	a.VerificationKeys, err = coreauth.NewJWKSCache(a.Issuer)
	if err != nil {
		return cfg, err
	}
	a.ClientSecret, err = secretFromEnv("SB0RKA_IR_WEB_CLIENT_SECRET")
	if err != nil {
		return cfg, err
	}
	a.SessionKey, err = keyFromEnv("IR_SESSION_ENCRYPTION_KEY")
	if err != nil {
		return cfg, err
	}
	a.ContextKey, err = keyFromEnv("IR_OIDC_CONTEXT_ENCRYPTION_KEY")
	if err != nil {
		return cfg, err
	}
	if bytes.Equal(a.SessionKey, a.ContextKey) {
		return cfg, fmt.Errorf("IR session and OIDC context encryption keys must differ")
	}
	if strings.TrimSpace(os.Getenv("IR_GATEWAY_CONFIGURATION_KEY")) != "" {
		cfg.Gateway.ConfigurationKey, err = keyFromEnv("IR_GATEWAY_CONFIGURATION_KEY")
		if err != nil {
			return cfg, err
		}
		if bytes.Equal(cfg.Gateway.ConfigurationKey, a.SessionKey) || bytes.Equal(cfg.Gateway.ConfigurationKey, a.ContextKey) {
			return cfg, fmt.Errorf("IR gateway configuration key must differ from authentication keys")
		}
	}
	if cfg.Database.URI == "" || a.ClientID == "" {
		return cfg, fmt.Errorf("DATABASE_URI and OIDC_CLIENT_ID are required")
	}
	if cfg.Server.CORSWhitelist["*"] {
		return cfg, fmt.Errorf("IR authentication requires explicit CORS origins")
	}
	return cfg, nil
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

func keyFromEnv(name string) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("%s must be a base64-encoded 32-byte key", name)
	}
	return decoded, nil
}
