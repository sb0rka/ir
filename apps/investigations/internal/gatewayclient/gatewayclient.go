// Package gatewayclient exposes the bounded, read-only Gateway operations used
// by Investigation REST and MCP transports.
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
type SourceObjectRef = gatewaycontract.SourceObjectRef
type ResolveContextRequest = gatewaycontract.ResolveContextRequest
type ResolveContextResponse = gatewaycontract.ResolveContextResponse
type SearchEventsRequest = gatewaycontract.SearchEventsRequest
type SearchEventsResponse = gatewaycontract.SearchEventsResponse
type LookupEntityRequest = gatewaycontract.LookupEntityRequest
type LookupEntityResponse = gatewaycontract.LookupEntityResponse
type TimeRange = gatewaycontract.TimeRange
type EntityRef = gatewaycontract.EntityRef
type EventSort = gatewaycontract.EventSort

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

func (client *Client) SearchEvents(ctx context.Context, projectID, bearer string, body SearchEventsRequest) (SearchEventsResponse, error) {
	if err := client.ready(); err != nil {
		return SearchEventsResponse{}, err
	}
	response, err := client.api.SearchEventsWithResponse(ctx,
		&gatewaycontract.SearchEventsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	if err != nil {
		return SearchEventsResponse{}, fmt.Errorf("call Gateway event search: %w", err)
	}
	if response.StatusCode() != http.StatusOK {
		return SearchEventsResponse{}, parseHTTPError(response.StatusCode(), response.Body)
	}
	if response.JSON200 == nil {
		return SearchEventsResponse{}, fmt.Errorf("decode Gateway event search response: expected application/json")
	}
	return *response.JSON200, nil
}

func (client *Client) LookupEntity(ctx context.Context, projectID, bearer string, body LookupEntityRequest) (LookupEntityResponse, error) {
	if err := client.ready(); err != nil {
		return LookupEntityResponse{}, err
	}
	response, err := client.api.LookupEntityWithResponse(ctx,
		&gatewaycontract.LookupEntityParams{XProjectID: projectID}, body, bearerEditor(bearer))
	if err != nil {
		return LookupEntityResponse{}, fmt.Errorf("call Gateway entity lookup: %w", err)
	}
	if response.StatusCode() != http.StatusOK {
		return LookupEntityResponse{}, parseHTTPError(response.StatusCode(), response.Body)
	}
	if response.JSON200 == nil {
		return LookupEntityResponse{}, fmt.Errorf("decode Gateway entity lookup response: expected application/json")
	}
	return *response.JSON200, nil
}

func (client *Client) ready() error {
	if client == nil || client.api == nil {
		if client != nil && client.initErr != nil {
			return fmt.Errorf("initialize Gateway client: %w", client.initErr)
		}
		return ErrUnavailable
	}
	return nil
}

func bearerEditor(bearer string) gatewaycontract.RequestEditorFn {
	return func(_ context.Context, request *http.Request) error {
		if bearer = strings.TrimSpace(bearer); bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		return nil
	}
}

func parseHTTPError(status int, body []byte) error {
	var envelope gatewaycontract.ErrorEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return &HTTPError{Status: status, Code: "gateway_error", Message: "Gateway rejected the request"}
	}
	return &HTTPError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
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
