package config

import (
	"crypto/ed25519"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

// Своего только то, чего нет у соседей: проверка токена и роли SOC.
type Config struct {
	Server   ServerConfig
	Database coreconfig.DatabaseConfig
	Log      coreconfig.LoggerConfig
}

type ServerConfig struct {
	Addr string
	Port string

	// Формат core: "*" — обычный ключ карты, а не отдельный флаг.
	CORSWhitelist map[string]bool

	Auth AuthConfig

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
