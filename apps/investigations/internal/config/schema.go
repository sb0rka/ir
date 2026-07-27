package config

import (
	"crypto/ed25519"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Worker   WorkerConfig
	Log      LogConfig
}

type ServerConfig struct {
	Addr string
	Port string

	CORSWhitelist  map[string]bool
	CORSAllowedAll bool

	Auth       AuthConfig
	Pagination PaginationConfig

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

type PaginationConfig struct {
	DefaultLimit int
	MaxLimit     int
}

type DatabaseConfig struct {
	URI             string
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

type WorkerConfig struct {
	ID           string
	Kinds        []string
	PollInterval time.Duration
	LeaseTimeout time.Duration
	MaxAttempts  int
}

type LogConfig struct {
	Level  string
	Format string
}
