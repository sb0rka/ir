package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

func TestAgentSecretCreateAndRetryAppendUsePlatformVersions(t *testing.T) {
	var secret configurationSecret
	var values []string
	var tagCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer platform-credential" {
			t.Error("wrong Platform bearer")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /projects/aabbccddee/secrets":
			items := []configurationSecret{}
			if secret.SecretID != "" {
				items = append(items, secret)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": items})
		case "POST /projects/aabbccddee/secret":
			var body struct {
				Name  string `json:"name"`
				Kind  string `json:"payload_kind"`
				Value string `json:"secret_value"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name != AgentSecretName || body.Kind != "text" || !agentaccess.IsToken(body.Value) {
				t.Error("invalid text secret creation")
			}
			secret = configurationSecret{PublicID: "012345abcdef", SecretID: "00000000-0000-0000-0000-000000000001", Name: AgentSecretName, PayloadKind: "text", Version: 1, UpdatedAt: time.Now().UTC()}
			values = append(values, body.Value)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(secret)
		case "POST /projects/aabbccddee/resources/012345abcdef/tag":
			tagCalls++
			_, _ = w.Write([]byte(`{}`))
		case "POST /projects/aabbccddee/resources/012345abcdef/secret/versions":
			var body struct {
				Value string `json:"secret_value"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || !agentaccess.IsToken(body.Value) {
				t.Error("invalid new text secret version")
			}
			values = append(values, body.Value)
			secret.Version++
			_ = json.NewEncoder(w).Encode(map[string]any{"version_no": secret.Version, "created_at": time.Now().UTC()})
		default:
			t.Error("unexpected Platform call", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, _ := NewAgentSecretClient(server.URL)
	firstToken, _ := agentaccess.Generate()
	first, err := client.Write(context.Background(), "platform-credential", "aabbccddee", "", firstToken)
	if err != nil || first.VersionNo != 1 || first.SecretID != secret.SecretID {
		t.Fatal("initial creation failed", err)
	}
	// Retrying an activation after a lost local write finds the existing secret, without requiring its value.
	secondToken, _ := agentaccess.Generate()
	second, err := client.Write(context.Background(), "platform-credential", "aabbccddee", "", secondToken)
	if err != nil || second.VersionNo != 2 || second.SecretID != first.SecretID || tagCalls != 2 || values[0] != firstToken || values[1] != secondToken {
		t.Fatal("retry did not append to canonical secret", err)
	}
	if _, err := client.Write(context.Background(), "platform-credential", "aabbccddee", "00000000-0000-0000-0000-000000000002", secondToken); !errors.Is(err, agentaccess.ErrConflict) || len(values) != 2 {
		t.Fatal("unexpected replacement secret accepted", err)
	}
}

func TestAgentSecretRejectsDuplicateResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("duplicate resource caused write")
		}
		secret := configurationSecret{PublicID: "012345abcdef", SecretID: "00000000-0000-0000-0000-000000000001", Name: AgentSecretName, PayloadKind: "text", Version: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []configurationSecret{secret, secret}})
	}))
	defer server.Close()
	client, _ := NewAgentSecretClient(server.URL)
	token, _ := agentaccess.Generate()
	if _, err := client.Write(context.Background(), "bearer", "aabbccddee", "", token); !errors.Is(err, agentaccess.ErrConflict) {
		t.Fatal("duplicates accepted", err)
	}
}

func TestResolvedConfigurationDoesNotContactPlatform(t *testing.T) {
	client, _ := NewConfigurationClient("http://127.0.0.1:1")
	document := irconfig.Document{Value: `[{"name":"pt-nad","sessionid":"server-copy"}]`, Values: map[string]map[string]string{"pt-nad": {"sessionid": "server-copy"}}}
	ctx := WithResolvedConfiguration(context.Background(), "agent-token", "aabbccddee", document)
	loaded, err := client.Snapshot(ctx, "agent-token", "aabbccddee")
	if err != nil || loaded.Value != document.Value {
		t.Fatal("preloaded snapshot not used", err)
	}
	if _, err := client.Snapshot(ctx, "other-token", "aabbccddee"); err == nil {
		t.Fatal("preloaded snapshot reused for another credential")
	}
	if _, err := client.Snapshot(ctx, "agent-token", "1122334455"); err == nil {
		t.Fatal("preloaded snapshot crossed project")
	}
}
