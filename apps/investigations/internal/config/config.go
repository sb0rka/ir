package config

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	coreconfig "github.com/sb0rka/sb0rka/packages/core/config"
)

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Addr:                   coreconfig.GetStringEnv("SERVER_ADDR", "0.0.0.0"),
			Port:                   coreconfig.GetStringEnv("SERVER_PORT", "8090"),
			CORSWhitelist:          coreconfig.ParseCORSWhitelist(coreconfig.GetStringEnv("SERVER_CORS_WHITELIST", "")),
			BootstrapAdminSubjects: envList("INV_BOOTSTRAP_ADMIN_SUBJECTS"),
			DefaultRole:            coreconfig.GetStringEnv("INV_DEFAULT_ROLE", ""),
			Auth: AuthConfig{
				AccessTokenIssuer:   coreconfig.GetStringEnv("ACCESS_TOKEN_ISSUER", ""),
				AccessTokenAudience: coreconfig.GetStringEnv("ACCESS_TOKEN_AUDIENCE", ""),
				AccessTokenKid:      coreconfig.GetStringEnv("ACCESS_TOKEN_KID", ""),
				AccessTokenTyp:      coreconfig.GetStringEnv("ACCESS_TOKEN_TYP", "access+jwt"),
			},
		},
		Database: coreconfig.DatabaseConfig{
			URI:      coreconfig.GetStringEnv("DATABASE_URI", ""),
			MaxConns: coreconfig.GetIntEnv("DATABASE_MAX_CONNS", coreconfig.DefaultDatabaseMaxConns),
			ConnMaxLifetime: coreconfig.GetDurationEnv(
				"DATABASE_CONN_MAX_LIFETIME_SEC", coreconfig.DefaultDatabaseConnMaxLifetime, time.Second),
		},
		Log: coreconfig.LoggerConfig{
			Level:  coreconfig.GetStringEnv("LOG_LEVEL", "info"),
			Format: coreconfig.GetStringEnv("LOG_FORMAT", "json"),
		},
	}

	key, err := loadPublicKey()
	if err != nil {
		return cfg, err
	}
	cfg.Server.Auth.AccessTokenPublicKey = key

	if cfg.Database.URI == "" {
		return cfg, fmt.Errorf("DATABASE_URI is required")
	}
	return cfg, nil
}

// loadPublicKey читает публичный ключ платформы из PEM-файла или base64-переменной.
// Отсутствие ключа не ошибка старта: сервис поднимется и отдаст /healthz,
// но любой запрос с токеном получит 401 — так виден неполный конфиг стенда.
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
	if block == nil {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, fmt.Errorf("public key is neither PEM nor base64: %w", err)
		}
		raw = decoded
	} else {
		raw = block.Bytes
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

// envList — списковых переменных в core пока нет; когда понадобятся второму
// сервису, помощник переезжает туда.
func envList(key string) []string {
	raw := coreconfig.GetStringEnv(key, "")
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
