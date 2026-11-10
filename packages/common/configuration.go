package common

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sb0rka/ir/packages/common/irconfig"
)

type ConfigurationClient struct{ client *SecretsClient }

type configurationSecret struct {
	PublicID    string    `json:"public_id"`
	SecretID    string    `json:"secret_id"`
	Name        string    `json:"name"`
	PayloadKind string    `json:"payload_kind"`
	Version     int32     `json:"current_version_no"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewConfigurationClient(baseURL string) (*ConfigurationClient, error) {
	client, err := NewSecretsClient(SecretsConfig{BaseURL: baseURL, MaxResponseBytes: irconfig.MaxRequestBytes})
	if err != nil {
		return nil, err
	}
	return &ConfigurationClient{client: client}, nil
}

func (c *ConfigurationClient) find(ctx context.Context, bearer, project string) (configurationSecret, error) {
	var listed struct {
		Secrets []configurationSecret `json:"secrets"`
	}
	if err := c.client.doJSON(ctx, bearer, http.MethodGet, []string{"projects", project, "secrets"}, "list secrets", nil, &listed); err != nil {
		return configurationSecret{}, err
	}
	for _, secret := range listed.Secrets {
		if secret.Name != irconfig.SecretName {
			continue
		}
		if len(secret.PublicID) != 12 || secret.SecretID == "" || secret.Version < 1 {
			return configurationSecret{}, errors.New("invalid secret metadata")
		}
		for _, ch := range secret.PublicID {
			if ch < '0' || ch > '9' && ch < 'a' || ch > 'f' {
				return configurationSecret{}, errors.New("invalid secret metadata")
			}
		}
		return secret, nil
	}
	return configurationSecret{}, nil
}

func secretConfigurationMetadata(secret configurationSecret) irconfig.Metadata {
	if secret.Version == 0 {
		return irconfig.Metadata{}
	}
	return irconfig.Metadata{SecretID: secret.SecretID, Configured: true, Version: secret.Version, UpdatedAt: &secret.UpdatedAt}
}

func (c *ConfigurationClient) Read(ctx context.Context, bearer, project string) (irconfig.Document, error) {
	secret, err := c.find(ctx, bearer, project)
	if err != nil {
		return irconfig.Document{}, err
	}
	if secret.Version == 0 {
		return irconfig.Document{Value: "[]", Values: map[string]map[string]string{}}, nil
	}
	// Existing text secrets remain readable; new configurations use the JSON kind.
	if secret.PayloadKind != "text" && secret.PayloadKind != "json" {
		return irconfig.Document{}, &HTTPError{Op: "secret format", StatusCode: http.StatusConflict}
	}
	var revealed struct {
		SecretID string `json:"secret_id"`
		Version  int32  `json:"version_no"`
		Value    string `json:"value"`
	}
	path := []string{"projects", project, "resources", secret.PublicID, "secret", "versions", strconv.FormatInt(int64(secret.Version), 10), "reveal"}
	if err := c.client.doJSON(ctx, bearer, http.MethodPost, path, "reveal secret version", nil, &revealed); err != nil {
		return irconfig.Document{}, err
	}
	if revealed.SecretID != secret.SecretID || revealed.Version != secret.Version {
		return irconfig.Document{}, errors.New("secret version changed")
	}
	values, err := irconfig.Parse([]byte(revealed.Value))
	if err != nil {
		return irconfig.Document{}, &HTTPError{Op: "secret format", StatusCode: http.StatusConflict}
	}
	return irconfig.Document{Metadata: secretConfigurationMetadata(secret), Value: revealed.Value, Values: values}, nil
}

func (c *ConfigurationClient) Put(ctx context.Context, bearer, project string, raw []byte) (irconfig.Metadata, error) {
	_, err := irconfig.Parse(raw)
	if err != nil {
		return irconfig.Metadata{}, &HTTPError{Op: "configuration", StatusCode: http.StatusBadRequest}
	}
	secret, err := c.find(ctx, bearer, project)
	if err != nil {
		return irconfig.Metadata{}, err
	}
	created := secret.Version == 0
	if created {
		err := c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "secret"}, "create secret", struct {
			Name        string `json:"name"`
			PayloadKind string `json:"payload_kind"`
			Value       string `json:"secret_value"`
		}{irconfig.SecretName, "json", string(raw)}, &secret)
		if err != nil {
			return irconfig.Metadata{}, err
		}
	}
	// Attach is idempotent; retrying a save also repairs a previously failed tag attachment.
	if err := c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "resources", secret.PublicID, "tag"}, "tag source configuration", struct {
		Key   string `json:"tag_key"`
		Value string `json:"tag_value"`
	}{"source", "ir"}, &struct{}{}); err != nil {
		return irconfig.Metadata{}, err
	}
	if created {
		return secretConfigurationMetadata(secret), nil
	}
	var version struct {
		Number    int32     `json:"version_no"`
		CreatedAt time.Time `json:"created_at"`
	}
	err = c.client.doJSON(ctx, bearer, http.MethodPost, []string{"projects", project, "resources", secret.PublicID, "secret", "versions"}, "create secret version", struct {
		Value string `json:"secret_value"`
	}{string(raw)}, &version)
	return irconfig.Metadata{SecretID: secret.SecretID, Configured: err == nil, Version: version.Number, UpdatedAt: &version.CreatedAt}, err
}

type configurationMemoKey struct{}
type configurationMemo struct {
	mu         sync.Mutex
	loaded     bool
	project    string
	bearerHash [32]byte
	document   irconfig.Document
	err        error
}

// WithConfigurationSnapshot confines plaintext reuse to one incoming request.
func WithConfigurationSnapshot(ctx context.Context) context.Context {
	return context.WithValue(ctx, configurationMemoKey{}, &configurationMemo{})
}

func WithResolvedConfiguration(ctx context.Context, bearer, project string, document irconfig.Document) context.Context {
	return context.WithValue(ctx, configurationMemoKey{}, &configurationMemo{
		loaded: true, project: project, bearerHash: sha256.Sum256([]byte(bearer)), document: document,
	})
}

func (c *ConfigurationClient) Snapshot(ctx context.Context, bearer, project string) (irconfig.Document, error) {
	memo, ok := ctx.Value(configurationMemoKey{}).(*configurationMemo)
	if !ok {
		return c.Read(ctx, bearer, project)
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if !memo.loaded {
		memo.document, memo.err = c.Read(ctx, bearer, project)
		memo.project, memo.bearerHash, memo.loaded = project, sha256.Sum256([]byte(bearer)), true
	}
	if memo.project != project || memo.bearerHash != sha256.Sum256([]byte(bearer)) {
		return irconfig.Document{}, fmt.Errorf("configuration project changed within request")
	}
	return memo.document, memo.err
}
