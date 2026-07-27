package config

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Addr:                   env("SERVER_ADDR", "0.0.0.0"),
			Port:                   env("SERVER_PORT", "8090"),
			BootstrapAdminSubjects: envList("INV_BOOTSTRAP_ADMIN_SUBJECTS"),
			DefaultRole:            env("INV_DEFAULT_ROLE", ""),
			Pagination: PaginationConfig{
				DefaultLimit: envInt("PAGE_DEFAULT_LIMIT", 50),
				MaxLimit:     envInt("PAGE_MAX_LIMIT", 200),
			},
			Auth: AuthConfig{
				AccessTokenIssuer:   env("ACCESS_TOKEN_ISSUER", ""),
				AccessTokenAudience: env("ACCESS_TOKEN_AUDIENCE", ""),
				AccessTokenKid:      env("ACCESS_TOKEN_KID", ""),
				AccessTokenTyp:      env("ACCESS_TOKEN_TYP", "access+jwt"),
			},
		},
		Database: DatabaseConfig{
			URI:             env("DATABASE_URI", ""),
			MaxOpenConns:    envInt("DATABASE_MAX_OPEN_CONNS", 10),
			ConnMaxLifetime: time.Duration(envInt("DATABASE_CONN_MAX_LIFETIME_SEC", 30)) * time.Second,
		},
		Worker: WorkerConfig{
			ID:           env("WORKER_ID", ""),
			Kinds:        envList("WORKER_KINDS"),
			PollInterval: time.Duration(envInt("WORKER_POLL_INTERVAL_SEC", 2)) * time.Second,
			LeaseTimeout: time.Duration(envInt("WORKER_LEASE_TIMEOUT_SEC", 300)) * time.Second,
			MaxAttempts:  envInt("WORKER_MAX_ATTEMPTS", 3),
		},
		Log: LogConfig{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "json"),
		},
	}

	cfg.Server.CORSWhitelist, cfg.Server.CORSAllowedAll = parseCORS(env("SERVER_CORS_WHITELIST", ""))

	key, err := loadPublicKey()
	if err != nil {
		return cfg, err
	}
	cfg.Server.Auth.AccessTokenPublicKey = key

	if cfg.Database.URI == "" {
		return cfg, fmt.Errorf("DATABASE_URI is required")
	}
	if cfg.Server.Pagination.DefaultLimit > cfg.Server.Pagination.MaxLimit {
		return cfg, fmt.Errorf("PAGE_DEFAULT_LIMIT exceeds PAGE_MAX_LIMIT")
	}
	return cfg, nil
}

// loadPublicKey читает публичный ключ платформы из PEM-файла или base64-переменной.
// Отсутствие ключа не ошибка старта: сервис поднимется и отдаст /health,
// но любой запрос с токеном получит 401 — так виден неполный конфиг стенда.
func loadPublicKey() (ed25519.PublicKey, error) {
	raw := []byte(env("ACCESS_TOKEN_PUBLIC_KEY", ""))
	if path := env("ACCESS_TOKEN_PUBLIC_KEY_FILE_PATH", ""); path != "" {
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

func parseCORS(raw string) (map[string]bool, bool) {
	list := make(map[string]bool)
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			return list, true
		}
		if origin != "" {
			list[origin] = true
		}
	}
	return list, false
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envList(key string) []string {
	raw := env(key, "")
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
