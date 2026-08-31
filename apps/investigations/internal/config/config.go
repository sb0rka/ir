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
			Addr:          coreconfig.GetStringEnv("SERVER_ADDR", "0.0.0.0"),
			Port:          coreconfig.GetStringEnv("SERVER_PORT", "8090"),
			CORSWhitelist: coreconfig.ParseCORSWhitelist(coreconfig.GetStringEnv("SERVER_CORS_WHITELIST", "")),
			Auth: AuthConfig{
				Disabled:            coreconfig.GetBoolEnv("AUTH_DISABLED", false),
				AccessTokenIssuer:   coreconfig.GetStringEnv("ACCESS_TOKEN_ISSUER", ""),
				AccessTokenAudience: coreconfig.GetStringEnv("ACCESS_TOKEN_AUDIENCE", "api.local"),
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
		Platform: PlatformConfig{
			APIBaseURL: strings.TrimRight(coreconfig.GetStringEnv("SB0RKA_API_BASE_URL", ""), "/"),
		},
		SOM: SOMConfig{
			APIBaseURL:     strings.TrimRight(coreconfig.GetStringEnv("SOM_API_BASE_URL", ""), "/"),
			RelayBaseURL:   strings.TrimRight(coreconfig.GetStringEnv("SOM_RELAY_BASE_URL", ""), "/"),
			HostID:         coreconfig.GetStringEnv("SOM_HOST_ID", ""),
			RepoID:         coreconfig.GetStringEnv("SOM_REPO_ID", ""),
			RepoParentPath: coreconfig.GetStringEnv("SOM_REPO_PARENT_PATH", "/tmp"),
			RepoFolderName: coreconfig.GetStringEnv("SOM_REPO_FOLDER_NAME", "ir-demo"),
			TargetBranch:   coreconfig.GetStringEnv("SOM_TARGET_BRANCH", "main"),
			Executor:       coreconfig.GetStringEnv("SOM_EXECUTOR", "OPENCODE"),
		},
		Gateway: GatewayConfig{
			BaseURL: strings.TrimRight(coreconfig.GetStringEnv("GATEWAY_BASE_URL", ""), "/"),
		},
	}
	// Публичные адреса по умолчанию совпадают с внутренними: на стенде,
	// где daemon VM видит те же имена, лишние переменные не нужны.
	cfg.Prompt = PromptConfig{
		IRBaseURL: strings.TrimRight(coreconfig.GetStringEnv(
			"IR_PUBLIC_BASE_URL", "http://localhost:"+cfg.Server.Port), "/"),
		AllowInsecureMCPHTTP: coreconfig.GetBoolEnv("MCP_ALLOW_INSECURE_HTTP", false),
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

// loadPublicKey добывает ключ проверки. Публичный берётся как есть; приватный
// принимается, потому что на стенде auth и ir поднимаются из одной пары, и
// раздавать два файла вместо одного незачем — публичная половина выводится
// из приватной. Подписывать сервис всё равно ничего не умеет.
//
// Отсутствие ключа не ошибка старта: сервис поднимется и отдаст /healthz,
// но любой запрос с токеном получит 401 — так виден неполный конфиг стенда.
func loadPublicKey() (ed25519.PublicKey, error) {
	if key, err := loadPrivateKey(); err != nil || key != nil {
		return key, err
	}

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

// loadPrivateKey возвращает публичную половину пары, если задан приватный ключ.
// nil без ошибки означает «не задан» — дальше ищется публичный.
//
// Формат как у sb0rka Auth: ACCESS_TOKEN_PRIVATE_KEY — base64(PKCS#8 PEM).
// Дополнительно принимаются PEM/plain base64(DER) в env и файле.
func loadPrivateKey() (ed25519.PublicKey, error) {
	raw := []byte(coreconfig.GetStringEnv("ACCESS_TOKEN_PRIVATE_KEY", ""))
	if path := coreconfig.GetStringEnv("ACCESS_TOKEN_PRIVATE_KEY_FILE_PATH", ""); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		raw = data
	}
	if len(raw) == 0 {
		return nil, nil
	}

	der, err := decodePrivateKeyDER(raw)
	if err != nil {
		return nil, err
	}

	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ed25519")
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("cannot derive public key")
	}
	return public, nil
}

func decodePrivateKeyDER(raw []byte) ([]byte, error) {
	block, _ := pem.Decode(raw)
	if block != nil {
		return block.Bytes, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("private key is neither PEM nor base64: %w", err)
	}

	block, _ = pem.Decode(decoded)
	if block != nil {
		return block.Bytes, nil
	}

	return decoded, nil
}
