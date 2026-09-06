package wazuh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
)

const (
	SourceCode       = "wazuh"
	maxResponseBytes = int64(4 << 20)
	searchTimeout    = "8s"
)

type ClientConfig struct {
	HTTP             proxy.HTTPClientConfig
	Username         string
	Password         string
	IndexPattern     string
	MaxResponseBytes int64
	Now              func() time.Time
}

type Client struct {
	http             *proxy.HTTPClient
	username         string
	password         string
	indexPattern     string
	maxResponseBytes int64
	now              func() time.Time
}

func NewClient(cfg ClientConfig) (*Client, error) {
	username := strings.TrimSpace(cfg.Username)
	password := strings.TrimSpace(cfg.Password)
	if username == "" || password == "" {
		return nil, fmt.Errorf("Wazuh username and password are required")
	}
	if strings.ContainsAny(username, "\r\n") || strings.ContainsAny(password, "\r\n") {
		return nil, fmt.Errorf("Wazuh credentials must not contain line breaks")
	}
	pattern := strings.TrimSpace(cfg.IndexPattern)
	if pattern == "" {
		pattern = "wazuh-alerts-*"
	}
	if err := validateIndexPattern(pattern); err != nil {
		return nil, err
	}
	backend, err := proxy.NewHTTPClient(cfg.HTTP)
	if err != nil {
		return nil, fmt.Errorf("configure Wazuh transport: %w", err)
	}
	httpClient := *backend.Client
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	backend.Client = &httpClient

	limit := cfg.MaxResponseBytes
	if limit == 0 {
		limit = maxResponseBytes
	}
	if limit < 1 {
		return nil, fmt.Errorf("max response bytes must be positive")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Client{
		http:             backend,
		username:         username,
		password:         password,
		indexPattern:     pattern,
		maxResponseBytes: limit,
		now:              now,
	}, nil
}

func (client *Client) Search(ctx context.Context, body searchRequest) (searchResponse, error) {
	var response searchResponse
	path := strings.Trim(client.indexPattern, "/") + "/_search"
	if err := client.doJSON(ctx, "event search", http.MethodPost, path, body, &response); err != nil {
		return searchResponse{}, err
	}
	return response, nil
}

func (client *Client) GetDocument(ctx context.Context, index, documentID string) (getDocumentResponse, error) {
	if err := validateResolvedIndex(index, client.indexPattern); err != nil {
		return getDocumentResponse{}, err
	}
	if err := validateDocumentID(documentID); err != nil {
		return getDocumentResponse{}, err
	}
	var response getDocumentResponse
	path := url.PathEscape(index) + "/_doc/" + url.PathEscape(documentID)
	err := client.doJSON(ctx, "event detail", http.MethodGet, path, nil, &response)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return getDocumentResponse{}, &NotFoundError{ExternalID: index + "/" + documentID}
		}
		return getDocumentResponse{}, err
	}
	if !response.Found {
		return getDocumentResponse{}, &NotFoundError{ExternalID: index + "/" + documentID}
	}
	return response, nil
}

func (client *Client) ClusterHealth(ctx context.Context) (clusterHealthResponse, error) {
	var response clusterHealthResponse
	if err := client.doJSON(ctx, "cluster health", http.MethodGet, "_cluster/health", nil, &response); err != nil {
		return clusterHealthResponse{}, err
	}
	return response, nil
}

func (client *Client) doJSON(ctx context.Context, operation, method, requestPath string, payload any, destination any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &RequestError{Operation: operation, Message: "request payload is invalid"}
		}
		body = bytes.NewReader(encoded)
	}
	reference := &url.URL{Path: requestPath}
	target := client.http.BaseURL.ResolveReference(reference)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return &RequestError{Operation: operation, Message: "request could not be built"}
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.SetBasicAuth(client.username, client.password)

	response, err := client.http.Client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		transportError := &TransportError{Operation: operation}
		var networkError net.Error
		if errors.As(err, &networkError) {
			transportError.TimedOut = networkError.Timeout()
			transportError.TemporaryFailure = networkError.Temporary()
		}
		return transportError
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
	if err != nil {
		return &TransportError{Operation: operation}
	}
	if int64(len(responseBody)) > client.maxResponseBytes {
		return &ResponseError{Operation: operation, Message: "response exceeds the configured size limit"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{Operation: operation, StatusCode: response.StatusCode}
	}
	if destination == nil {
		return nil
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return &ResponseError{Operation: operation, Message: "response body is empty"}
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return &ResponseError{Operation: operation, Message: "response is not valid JSON"}
	}
	return nil
}

func validateIndexPattern(pattern string) error {
	if pattern == "" || strings.ContainsAny(pattern, "/, \t\r\n") || strings.Contains(pattern, "..") {
		return fmt.Errorf("Wazuh index pattern is invalid")
	}
	if !strings.HasPrefix(pattern, "wazuh-alerts-") {
		return fmt.Errorf("Wazuh index pattern must start with wazuh-alerts-")
	}
	return nil
}

func validateResolvedIndex(index, pattern string) error {
	index = strings.TrimSpace(index)
	if index == "" || strings.ContainsAny(index, "/, \t\r\n*?") || strings.Contains(index, "..") {
		return &RequestError{Operation: "event detail", Message: "source event index is invalid"}
	}
	prefix := strings.TrimSuffix(pattern, "*")
	if prefix == "" || !strings.HasPrefix(index, prefix) {
		return &RequestError{Operation: "event detail", Message: "source event index is outside the configured alerts pattern"}
	}
	return nil
}

func validateDocumentID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 512 || strings.ContainsAny(id, "/\r\n\x00") {
		return &RequestError{Operation: "event detail", Message: "source event document id is invalid"}
	}
	return nil
}
