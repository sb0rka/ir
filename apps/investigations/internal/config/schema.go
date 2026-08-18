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
	SOM      SOMConfig
	Gateway  GatewayConfig
	Prompt   PromptConfig
}

// GatewayConfig — внутренний адрес для получения выбранных записей по их
// исходным идентификаторам.
type GatewayConfig struct {
	BaseURL string
}

// SOMConfig — демо-интеграция с SOM: kanban API, relay и daemon-хост.
// Пустой APIBaseURL выключает som-домен, сервис при этом стартует.
type SOMConfig struct {
	APIBaseURL   string
	RelayBaseURL string
	// HostID — единственная ручная daemon VM демо-стенда.
	HostID string
	// RepoID необязателен: без него репозиторий ищется по имени папки
	// или создаётся git init'ом на хосте.
	RepoID         string
	RepoParentPath string
	RepoFolderName string
	TargetBranch   string
	Executor       string
}

// PromptConfig — адреса, которые подставляются агенту в prompt при запуске
// issue: с daemon VM сервисы видны не по тем же URL, что с localhost.
type PromptConfig struct {
	IRBaseURL      string
	GatewayBaseURL string
}

type ServerConfig struct {
	Addr string
	Port string

	// Формат core: "*" — обычный ключ карты, а не отдельный флаг.
	CORSWhitelist map[string]bool

	Auth AuthConfig
}

type AuthConfig struct {
	// Demo-режим: подпись токена не проверяется и роли не читаются.
	// Authorization при этом всё равно кладётся в контекст — som-домен
	// пробрасывает его в SOM как есть.
	Disabled bool

	// Сервис только проверяет токены платформы, поэтому держит публичный ключ.
	// Приватный ключ верификатору не нужен.
	AccessTokenPublicKey ed25519.PublicKey
	AccessTokenIssuer    string
	AccessTokenAudience  string
	AccessTokenKid       string
	AccessTokenTyp       string
}
