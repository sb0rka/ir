// Package authclient obtains short-lived investigation-scoped agent JWTs from
// Sb0rka Auth. The user's access token is used only for this delegation call.
package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxResponseBody = 1 << 20

var ErrUnavailable = errors.New("Sb0rka Auth is not configured")

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	baseURL string
	http    *http.Client
}

type HTTPError struct {
	Status int
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("agent token request failed with status %d", err.Status)
}

func New(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), http: httpClient}
}

func (client *Client) InvestigationToken(ctx context.Context, bearer, projectID, investigationID string) (string, error) {
	if client == nil || client.baseURL == "" {
		return "", ErrUnavailable
	}
	body, err := json.Marshal(map[string]string{
		"project_id":       projectID,
		"investigation_id": investigationID,
	})
	if err != nil {
		return "", fmt.Errorf("encode agent token request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		client.baseURL+"/auth/agent-tokens/investigation", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create agent token request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearer))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("call Sb0rka Auth: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return "", fmt.Errorf("read agent token response: %w", err)
	}
	if len(raw) > maxResponseBody {
		return "", errors.New("agent token response is too large")
	}
	if response.StatusCode != http.StatusOK {
		return "", &HTTPError{Status: response.StatusCode}
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &result) != nil || strings.TrimSpace(result.AccessToken) == "" ||
		!strings.EqualFold(result.TokenType, "Bearer") || result.ExpiresIn <= 0 {
		return "", errors.New("Sb0rka Auth returned an invalid agent token response")
	}
	return result.AccessToken, nil
}
