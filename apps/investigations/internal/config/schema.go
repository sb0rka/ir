package config

import (
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

// Config combines shared infrastructure settings with IR integrations.
type Config struct {
	Server   ServerConfig
	Database coreconfig.DatabaseConfig
	Log      coreconfig.LoggerConfig
	Platform PlatformConfig
	Gateway  GatewayConfig
}

type PlatformConfig struct {
	APIBaseURL string
}

// GatewayConfig — внутренний адрес для получения выбранных записей по их
// исходным идентификаторам.
type GatewayConfig struct {
	BaseURL string
}

type ServerConfig struct {
	Addr string
	Port string

	// Формат core: "*" — обычный ключ карты, а не отдельный флаг.
	CORSWhitelist map[string]bool

	Auth AuthConfig
}

type AuthConfig struct {
	Issuer           string
	ClientID         string
	ClientSecret     []byte
	PublicBaseURL    string
	WebURL           string
	CookieSecure     bool
	AllowHTTP        bool
	SessionKey       []byte
	ContextKey       []byte
	VerificationKeys *coreauth.JWKSCache
}
