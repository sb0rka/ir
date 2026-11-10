package common

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sb0rka/ir/packages/common/agentaccess"
)

const AgentSecretName = "IR/AGENT_ACCESS"

type SecretReference struct {
	SecretID  string
	PublicID  string
	VersionNo int32
	CreatedAt time.Time
}

type AgentSecretClient struct{ client *SecretsClient }

func NewAgentSecretClient(baseURL string) (*AgentSecretClient, error) {
	client, err := NewSecretsClient(SecretsConfig{BaseURL: baseURL})
	if err != nil {
		return nil, err
	}
	return &AgentSecretClient{client: client}, nil
}

func validSecretReference(secret configurationSecret) bool {
	if len(secret.PublicID) != 12 || len(secret.SecretID) != 36 || secret.Version < 1 {
		return false
	}
	if _, err := hex.DecodeString(secret.PublicID); err != nil {
		return false
	}
	for _, position := range []int{8, 13, 18, 23} {
		if secret.SecretID[position] != '-' {
			return false
		}
	}
	_, err := hex.DecodeString(strings.ReplaceAll(secret.SecretID, "-", ""))
	return err == nil && len(strings.ReplaceAll(secret.SecretID, "-", "")) == 32
}

func (c *AgentSecretClient) Write(ctx context.Context, bearer, project, expectedSecretID, raw string) (SecretReference, error) {
	if !agentaccess.IsToken(raw) {
		return SecretReference{}, errors.New("invalid agent secret format")
	}
	var listed struct {
		Secrets []configurationSecret `json:"secrets"`
	}
	if err := c.client.doJSON(ctx, bearer, http.MethodGet, []string{"projects", project, "secrets"}, "list agent secrets", nil, &listed); err != nil {
		return SecretReference{}, err
	}
	var found configurationSecret
	for _, secret := range listed.Secrets {
		if secret.Name != AgentSecretName {
			continue
		}
		if found.SecretID != "" {
			return SecretReference{}, agentaccess.ErrConflict
		}
		if !validSecretReference(secret) || secret.PayloadKind != "text" {
			return SecretReference{}, agentaccess.ErrConflict
		}
		found = secret
	}
	if expectedSecretID != "" && found.SecretID != expectedSecretID {
		return SecretReference{}, agentaccess.ErrConflict
	}
	created := found.SecretID == ""
	if created {
		if err := c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "secret"}, "create agent secret", struct {
			Name  string `json:"name"`
			Kind  string `json:"payload_kind"`
			Value string `json:"secret_value"`
		}{AgentSecretName, "text", raw}, &found); err != nil {
			return SecretReference{}, err
		}
		if !validSecretReference(found) {
			return SecretReference{}, errors.New("invalid created agent secret metadata")
		}
	}
	if err := c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "resources", found.PublicID, "tag"}, "tag agent secret", struct {
		Key   string `json:"tag_key"`
		Value string `json:"tag_value"`
	}{"source", "ir"}, &struct{}{}); err != nil {
		return SecretReference{}, err
	}
	ref := SecretReference{SecretID: found.SecretID, PublicID: found.PublicID, VersionNo: found.Version, CreatedAt: found.UpdatedAt}
	if created {
		return ref, nil
	}
	var version struct {
		VersionNo int32     `json:"version_no"`
		CreatedAt time.Time `json:"created_at"`
	}
	if err := c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "resources", found.PublicID, "secret", "versions"}, "rotate agent secret", struct {
		Value string `json:"secret_value"`
	}{raw}, &version); err != nil {
		return SecretReference{}, err
	}
	if version.VersionNo <= found.Version {
		return SecretReference{}, errors.New("invalid agent secret version metadata")
	}
	ref.VersionNo, ref.CreatedAt = version.VersionNo, version.CreatedAt
	return ref, nil
}
