package agentaccess

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

const Prefix = "s0_ir_agent_"

var (
	ErrNotFound             = errors.New("agent access not found")
	ErrUnauthorized         = errors.New("invalid agent access token")
	ErrConflict             = errors.New("agent access version changed")
	ErrConfigurationMissing = errors.New("gateway configuration is not available")
)

type Record struct {
	ProjectID string
	SecretID  string
	VersionNo int32
	DigestAlg string
	Digest    []byte
	Prefix    string
	Postfix   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Configuration struct {
	ProjectID       string
	SecretID        string
	VersionNo       int32
	EncryptedConfig []byte
	UpdatedAt       time.Time
}

func Generate() (string, error) {
	var entropy [39]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(entropy[:]), nil
}

func Valid(raw string) bool {
	if len(raw) != 64 || !strings.HasPrefix(raw, Prefix) {
		return false
	}
	for _, ch := range raw[len(Prefix):] {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func IsToken(raw string) bool { return Valid(raw) }

func Digest(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func Matches(raw string, digest []byte) bool {
	return Valid(raw) && len(digest) == sha256.Size && subtle.ConstantTimeCompare(Digest(raw), digest) == 1
}
