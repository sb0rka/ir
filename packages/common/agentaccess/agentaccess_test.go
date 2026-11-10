package agentaccess

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/packages/common/irconfig"
)

func TestTokenGenerationAndVerification(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		token, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if !IsToken(token) || len(token) != 64 || seen[token] {
			t.Fatal("invalid or repeated generated token")
		}
		seen[token] = true
		digest := Digest(token)
		if len(digest) != 32 || !Matches(token, digest) {
			t.Fatal("generated token did not verify")
		}
		mutated := append([]byte(nil), digest...)
		mutated[0] ^= 1
		if Matches(token, mutated) || Matches(token, digest[:31]) {
			t.Fatal("wrong digest accepted")
		}
	}
	for _, token := range []string{"", Prefix + strings.Repeat("x", 51), Prefix + strings.Repeat("x", 53), Prefix + strings.Repeat("x", 51) + "!", "S0_ir_agent_" + strings.Repeat("x", 52)} {
		if IsToken(token) || Matches(token, Digest(token)) {
			t.Fatal("malformed token accepted")
		}
	}
}

func TestConfigurationEncryptionBindsProjectAndDetectsTampering(t *testing.T) {
	c, err := NewCipher(bytes.Repeat([]byte{42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	document := irconfig.Document{Metadata: irconfig.Metadata{Configured: true, SecretID: "00000000-0000-0000-0000-000000000001", Version: 3, UpdatedAt: &now}, Value: `[{"name":"pt-nad","sessionid":"private-source-credential"}]`}
	sealed, err := c.Seal("aabbccddee", document)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.EncryptedConfig, []byte("private-source-credential")) {
		t.Fatal("plaintext stored in encrypted configuration")
	}
	opened, err := c.Open(sealed)
	if err != nil || opened.Value != document.Value || opened.Values["pt-nad"]["sessionid"] != "private-source-credential" || opened.SecretID != document.SecretID || opened.Version != 3 {
		t.Fatal("round trip failed", err)
	}
	wrongProject := sealed
	wrongProject.ProjectID = "1122334455"
	if _, err := c.Open(wrongProject); err == nil {
		t.Fatal("ciphertext crossed project boundary")
	}
	tampered := sealed
	tampered.EncryptedConfig = bytes.Clone(sealed.EncryptedConfig)
	tampered.EncryptedConfig[len(tampered.EncryptedConfig)-1] ^= 1
	if _, err := c.Open(tampered); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	second, err := c.Seal("aabbccddee", document)
	if err != nil || bytes.Equal(second.EncryptedConfig, sealed.EncryptedConfig) {
		t.Fatal("nonce reused", err)
	}
	encoded, err := json.Marshal(opened.Metadata)
	if err != nil || strings.Contains(string(encoded), document.SecretID) {
		t.Fatal("internal secret ID escaped to public metadata", err)
	}
}

func TestCipherRejectsInvalidConfigurationAndKeys(t *testing.T) {
	for _, length := range []int{0, 16, 24, 31, 33} {
		if _, err := NewCipher(make([]byte, length)); err == nil {
			t.Fatal("accepted invalid key length", length)
		}
	}
	c, _ := NewCipher(make([]byte, 32))
	if _, err := c.Seal("aabbccddee", irconfig.Document{Value: `{"name":"invalid"}`}); err == nil {
		t.Fatal("invalid document encrypted")
	}
	if _, err := c.Seal("aabbccddee", irconfig.Document{Metadata: irconfig.Metadata{Configured: true}, Value: `[]`}); err == nil {
		t.Fatal("configured document without source identity accepted")
	}
	sealed, err := c.Seal("aabbccddee", irconfig.Document{Value: `[]`})
	if err != nil {
		t.Fatal(err)
	}
	if document, err := c.Open(sealed); err != nil || document.Configured || len(document.Values) != 0 {
		t.Fatal("unconfigured empty snapshot failed", err)
	}
}

func TestCipherUsesObservationTimeForSnapshotOrdering(t *testing.T) {
	c, _ := NewCipher(make([]byte, 32))
	old := time.Unix(1, 0)
	started := time.Now().UTC()
	record, err := c.Seal("aabbccddee", irconfig.Document{
		Metadata: irconfig.Metadata{Configured: true, SecretID: "00000000-0000-0000-0000-000000000001", Version: 1, UpdatedAt: &old},
		Value:    `[]`,
	})
	if err != nil || record.UpdatedAt.Before(started) || record.UpdatedAt.After(time.Now().UTC()) {
		t.Fatal("snapshot ordering used source modification time", err)
	}
}
