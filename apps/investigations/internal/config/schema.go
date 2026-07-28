package config

import (
	"crypto/ed25519"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Log      LogConfig
}

type ServerConfig struct {
	Addr string
	Port string

	CORSWhitelist  map[string]bool
	CORSAllowedAll bool

	Auth       AuthConfig
	// BootstrapAdminSubjects — субъекты с ролью admin в обход role_bindings.
	// Без них deny-by-default отказывает всем, включая того, кто должен раздать роли.
	BootstrapAdminSubjects []string
	// DefaultRole — роль субъекта без записей в role_bindings.
	// Пусто = deny. В dev-стенде выставляется в l2, чтобы не раздавать роли руками.
	DefaultRole string
}

type AuthConfig struct {
	// Сервис только проверяет токены платформы, поэтому держит публичный ключ.
	// Приватный ключ верификатору не нужен.
	AccessTokenPublicKey ed25519.PublicKey
	AccessTokenIssuer    string
	AccessTokenAudience  string
	AccessTokenKid       string
	AccessTokenTyp       string
}

type DatabaseConfig struct {
	URI             string
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}
