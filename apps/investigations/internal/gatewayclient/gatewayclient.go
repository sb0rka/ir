// Package gatewayclient resolves source-owned record identifiers through the
// normalized Gateway API. It deliberately does not expose Gateway search: UI
// and SOM agents search independently and send only their selected references.
package gatewayclient

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

	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
)

const maxResponseBody = 16 << 20

var ErrUnavailable = errors.New("gateway is not configured")

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	api     *gatewaycontract.ClientWithResponses
	initErr error
}

type HTTPError struct {
	Status  int
	Code    string
	Message string
}

type EventSourceRef = gatewaycontract.EventSourceRef
type EntitySourceRef = gatewaycontract.EntitySourceRef
type ResolveContextRequest = gatewaycontract.ResolveContextRequest
type ResolveContextResponse = gatewaycontract.ResolveContextResponse

func (err *HTTPError) Error() string {
	return fmt.Sprintf("gateway request failed with status %d: %s", err.Status, err.Code)
}

func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		return &Client{}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	generated, err := gatewaycontract.NewClientWithResponses(
		baseURL,
		gatewaycontract.WithHTTPClient(responseLimitDoer{next: httpClient}),
	)
	return &Client{api: generated, initErr: err}
}

func (client *Client) ResolveContext(ctx context.Context, projectID, bearer string, body ResolveContextRequest) (ResolveContextResponse, error) {
	if client == nil || client.api == nil {
		if client != nil && client.initErr != nil {
			return ResolveContextResponse{}, fmt.Errorf("initialize Gateway client: %w", client.initErr)
		}
		return ResolveContextResponse{}, ErrUnavailable
	}
	params := &gatewaycontract.ResolveContextParams{XProjectID: projectID}
	var editors []gatewaycontract.RequestEditorFn
	if bearer = strings.TrimSpace(bearer); bearer != "" {
		editors = append(editors, func(_ context.Context, request *http.Request) error {
			request.Header.Set("Authorization", "Bearer "+bearer)
			return nil
		})
	}
	response, err := client.api.ResolveContextWithResponse(ctx, params, body, editors...)
	if err != nil {
		return ResolveContextResponse{}, fmt.Errorf("call Gateway context resolver: %w", err)
	}
	status := response.StatusCode()
	if status != http.StatusOK {
		var envelope gatewaycontract.ErrorEnvelope
		if json.Unmarshal(response.Body, &envelope) != nil {
			return ResolveContextResponse{}, &HTTPError{Status: status, Code: "gateway_error", Message: "Gateway rejected the request"}
		}
		return ResolveContextResponse{}, &HTTPError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	if response.JSON200 == nil {
		return ResolveContextResponse{}, fmt.Errorf("decode Gateway context response: expected application/json")
	}
	return *response.JSON200, nil
}

type responseLimitDoer struct {
	next gatewaycontract.HttpRequestDoer
}

func (doer responseLimitDoer) Do(request *http.Request) (*http.Response, error) {
	response, err := doer.next.Do(request)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	_ = response.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read Gateway response: %w", readErr)
	}
	if len(raw) > maxResponseBody {
		return nil, fmt.Errorf("Gateway response is too large")
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	response.ContentLength = int64(len(raw))
	return response, nil
}
