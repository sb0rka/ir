package agentaccess

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/sb0rka/ir/packages/common/irconfig"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("gateway configuration key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Seal(projectID string, document irconfig.Document) (Configuration, error) {
	if c == nil || c.aead == nil || projectID == "" {
		return Configuration{}, errors.New("gateway encryption is not configured")
	}
	if _, err := irconfig.Parse([]byte(document.Value)); err != nil {
		return Configuration{}, err
	}
	if document.Configured && (document.SecretID == "" || document.Version < 1) {
		return Configuration{}, errors.New("invalid source configuration metadata")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Configuration{}, err
	}
	return Configuration{ProjectID: projectID, SecretID: document.SecretID, VersionNo: document.Version,
		EncryptedConfig: c.aead.Seal(nonce, nonce, []byte(document.Value), []byte(projectID)), UpdatedAt: time.Now().UTC()}, nil
}

func (c *Cipher) Open(record Configuration) (irconfig.Document, error) {
	if c == nil || c.aead == nil {
		return irconfig.Document{}, errors.New("gateway encryption is not configured")
	}
	if record.ProjectID == "" || len(record.EncryptedConfig) < c.aead.NonceSize()+c.aead.Overhead() {
		return irconfig.Document{}, ErrConfigurationMissing
	}
	nonce := record.EncryptedConfig[:c.aead.NonceSize()]
	plain, err := c.aead.Open(nil, nonce, record.EncryptedConfig[c.aead.NonceSize():], []byte(record.ProjectID))
	if err != nil {
		return irconfig.Document{}, fmt.Errorf("decrypt gateway configuration: %w", err)
	}
	values, err := irconfig.Parse(plain)
	if err != nil {
		return irconfig.Document{}, errors.New("invalid encrypted gateway configuration")
	}
	return irconfig.Document{Metadata: irconfig.Metadata{SecretID: record.SecretID, Configured: record.VersionNo > 0,
		Version: record.VersionNo, UpdatedAt: &record.UpdatedAt}, Value: string(plain), Values: values}, nil
}
