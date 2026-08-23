// Package common contains small, shared integration primitives used by IR services.
package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSecretsTimeout   = 10 * time.Second
	defaultMaxResponseBytes = int64(1 << 20)
)

var (
	ErrSecretNotFound = errors.New("secret not found")
	ErrUnauthorized   = errors.New("sb0rka api rejected the access token")
	ErrForbidden      = errors.New("sb0rka api denied access to the project")
)

// SecretsConfig configures the project-scoped Sb0rka Secrets client.
type SecretsConfig struct {
	BaseURL          string
	Timeout          time.Duration
	MaxResponseBytes int64
}

// HTTPError reports an unexpected non-2xx response without retaining its body,
// which may contain information that must not reach service logs.
type HTTPError struct {
	Op         string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: sb0rka api returned status %d", e.Op, e.StatusCode)
}

// SecretValue is one revealed, immutable secret version.
type SecretValue struct {
	Name        string
	SecretID    string
	VersionNo   int
	PayloadKind string
	Value       string
}

// SecretSnapshot contains versions captured by one metadata read. Reveals use
// those exact version numbers, so concurrently created versions cannot produce
// a mixed configuration.
type SecretSnapshot struct {
	ProjectID string
	Values    map[string]SecretValue
}

func (s SecretSnapshot) Value(name string) (string, bool) {
	secret, ok := s.Values[name]
	return secret.Value, ok
}

type SecretsClient struct {
	baseURL          *url.URL
	http             *http.Client
	maxResponseBytes int64
}

func NewSecretsClient(cfg SecretsConfig) (*SecretsClient, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse SB0RKA_API_BASE_URL: %w", err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, errors.New("SB0RKA_API_BASE_URL must be an absolute http or https URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("SB0RKA_API_BASE_URL must not contain userinfo, query, or fragment")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultSecretsTimeout
	}
	maxResponseBytes := cfg.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}

	return &SecretsClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxResponseBytes: maxResponseBytes,
	}, nil
}

// ResolveSnapshot finds every requested name exactly, captures the current
// version numbers, then reveals those immutable versions. It is all-or-nothing.
func (c *SecretsClient) ResolveSnapshot(
	ctx context.Context,
	bearer string,
	projectID string,
	names ...string,
) (SecretSnapshot, error) {
	if c == nil {
		return SecretSnapshot{}, errors.New("secrets client is not configured")
	}
	bearer = strings.TrimSpace(bearer)
	projectID = strings.TrimSpace(projectID)
	if bearer == "" {
		return SecretSnapshot{}, errors.New("access token is required")
	}
	if projectID == "" {
		return SecretSnapshot{}, errors.New("project id is required")
	}
	requested, err := normalizeSecretNames(names)
	if err != nil {
		return SecretSnapshot{}, err
	}

	var listed struct {
		ProjectID string `json:"project_id"`
		Secrets   []struct {
			SecretID         string `json:"secret_id"`
			Name             string `json:"name"`
			CurrentVersionNo int    `json:"current_version_no"`
		} `json:"secrets"`
	}
	if err := c.doJSON(ctx, bearer, http.MethodGet,
		[]string{"projects", projectID, "secrets"}, "list secrets", &listed); err != nil {
		return SecretSnapshot{}, err
	}

	refs := make(map[string]secretVersionRef, len(requested))
	for _, secret := range listed.Secrets {
		if _, wanted := requested[secret.Name]; !wanted {
			continue
		}
		if _, duplicate := refs[secret.Name]; duplicate {
			return SecretSnapshot{}, fmt.Errorf("secret %q occurs more than once", secret.Name)
		}
		if strings.TrimSpace(secret.SecretID) == "" || secret.CurrentVersionNo < 1 {
			return SecretSnapshot{}, fmt.Errorf("secret %q has invalid current version metadata", secret.Name)
		}
		refs[secret.Name] = secretVersionRef{
			name:      secret.Name,
			secretID:  secret.SecretID,
			versionNo: secret.CurrentVersionNo,
		}
	}
	for name := range requested {
		if _, ok := refs[name]; !ok {
			return SecretSnapshot{}, fmt.Errorf("%w: %s", ErrSecretNotFound, name)
		}
	}

	snapshot := SecretSnapshot{ProjectID: projectID, Values: make(map[string]SecretValue, len(refs))}
	for _, name := range names {
		name = strings.TrimSpace(name)
		ref, ok := refs[name]
		if !ok {
			continue // duplicate requested name; reveal it only once
		}
		var revealed struct {
			ProjectID   string `json:"project_id"`
			SecretID    string `json:"secret_id"`
			VersionNo   int    `json:"version_no"`
			PayloadKind string `json:"payload_kind"`
			Value       string `json:"value"`
		}
		if err := c.doJSON(ctx, bearer, http.MethodPost, []string{
			"projects", projectID, "resources", ref.secretID, "secret", "versions",
			strconv.Itoa(ref.versionNo), "reveal",
		}, "reveal secret version", &revealed); err != nil {
			return SecretSnapshot{}, fmt.Errorf("resolve secret %q: %w", name, err)
		}
		if revealed.ProjectID != projectID || revealed.SecretID != ref.secretID || revealed.VersionNo != ref.versionNo {
			return SecretSnapshot{}, fmt.Errorf("secret %q reveal metadata does not match requested version", name)
		}
		snapshot.Values[name] = SecretValue{
			Name:        name,
			SecretID:    ref.secretID,
			VersionNo:   ref.versionNo,
			PayloadKind: revealed.PayloadKind,
			Value:       revealed.Value,
		}
		delete(refs, name)
	}
	return snapshot, nil
}

type secretVersionRef struct {
	name      string
	secretID  string
	versionNo int
}

func normalizeSecretNames(names []string) (map[string]struct{}, error) {
	if len(names) == 0 {
		return nil, errors.New("at least one secret name is required")
	}
	out := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, errors.New("secret name must not be empty")
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func (c *SecretsClient) doJSON(
	ctx context.Context,
	bearer string,
	method string,
	path []string,
	op string,
	out any,
) error {
	joined, err := url.JoinPath(c.baseURL.String(), path...)
	if err != nil {
		return fmt.Errorf("%s: build url: %w", op, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, joined, nil)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return ErrUnauthorized
		case http.StatusForbidden:
			return ErrForbidden
		case http.StatusNotFound:
			return ErrSecretNotFound
		default:
			return &HTTPError{Op: op, StatusCode: resp.StatusCode}
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%s: read response: %w", op, err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return fmt.Errorf("%s: response exceeds %d bytes", op, c.maxResponseBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", op, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("%s: decode response: %w", op, err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}
