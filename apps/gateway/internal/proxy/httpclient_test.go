package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewHTTPClientRejectsCredentialsInURL(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientConfig{BaseURL: "https://token@example.test", Timeout: time.Second})
	if err == nil {
		t.Fatal("expected URL credentials to be rejected")
	}
}

func TestNewHTTPClientReadsCredentialFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(path, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewHTTPClient(HTTPClientConfig{BaseURL: "https://example.test/api", CredentialFile: path, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if client.Credential != "secret" || client.BaseURL.String() != "https://example.test/api/" {
		t.Fatalf("unexpected client: %#v", client)
	}
}
