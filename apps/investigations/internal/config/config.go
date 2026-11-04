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
				Issuer:   strings.TrimRight(coreconfig.GetStringEnv("AUTH_ISSUER", ""), "/"),
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
	for _, origin := range []struct{ name, value string }{
		{"AUTH_ISSUER", a.Issuer},
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
	a.ClientSecret, err = readSecret("OIDC_CLIENT_SECRET_FILE_PATH")
	if err != nil {
		return cfg, err
	}
	a.SessionKey, err = readKey("IR_SESSION_ENCRYPTION_KEY_FILE_PATH")
	if err != nil {
		return cfg, err
	}
	a.ContextKey, err = readKey("IR_OIDC_CONTEXT_ENCRYPTION_KEY_FILE_PATH")
	if err != nil {
		return cfg, err
	}
	if bytes.Equal(a.SessionKey, a.ContextKey) {
		return cfg, fmt.Errorf("IR session and OIDC context encryption keys must differ")
	}
	if cfg.Database.URI == "" || a.ClientID == "" {
		return cfg, fmt.Errorf("DATABASE_URI and OIDC_CLIENT_ID are required")
	}
	if cfg.Server.CORSWhitelist["*"] {
		return cfg, fmt.Errorf("IR authentication requires explicit CORS origins")
	}
	// Secrets and SoM integrations are outside this authentication migration.
	// Their configuration is deliberately not loaded into the new flow.
	return cfg, nil
}

func readSecret(name string) ([]byte, error) {
	path := strings.TrimSpace(os.Getenv(name))
	if path == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	raw = bytes.TrimSuffix(raw, []byte("\n"))
	if len(raw) < 32 {
		return nil, fmt.Errorf("%s requires at least 32 bytes", name)
	}
	return raw, nil
}

func readKey(name string) ([]byte, error) {
	path := strings.TrimSpace(os.Getenv(name))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(raw) == 32 {
		return raw, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("%s must contain a raw or base64-encoded 32-byte key", name)
	}
	return decoded, nil
}
