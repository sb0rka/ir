package gatewayclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
)

type gatewayResponse interface {
	GetBody() []byte
	StatusCode() int
}

func (client *Client) ListSources(ctx context.Context, projectID, bearer string) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.ListSourcesWithResponse(ctx,
		&gatewaycontract.ListSourcesParams{XProjectID: projectID}, bearerEditor(bearer))
	return gatewayJSON("list sources", response, err)
}

func (client *Client) SearchEvents(ctx context.Context, projectID, bearer string, body gatewaycontract.SearchEventsRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.SearchEventsWithResponse(ctx,
		&gatewaycontract.SearchEventsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("search events", response, err)
}

func (client *Client) AggregateEvents(ctx context.Context, projectID, bearer string, body gatewaycontract.AggregateEventsRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.AggregateEventsWithResponse(ctx,
		&gatewaycontract.AggregateEventsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("aggregate events", response, err)
}

func (client *Client) LookupEntity(ctx context.Context, projectID, bearer string, body gatewaycontract.LookupEntityRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.LookupEntityWithResponse(ctx,
		&gatewaycontract.LookupEntityParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("lookup entity", response, err)
}

func (client *Client) SearchFindings(ctx context.Context, projectID, bearer string, body gatewaycontract.SearchFindingsRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.SearchFindingsWithResponse(ctx,
		&gatewaycontract.SearchFindingsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("search findings", response, err)
}

func (client *Client) GetFinding(ctx context.Context, projectID, bearer string, ref gatewaycontract.SourceObjectRef) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.GetFindingWithResponse(ctx, ref.SourceCode,
		gatewaycontract.GetFindingParamsKind(ref.RecordType), ref.ExternalId, &gatewaycontract.GetFindingParams{
			XProjectID: projectID, SourceInstance: ref.SourceInstance, From: ref.TimeRange.From, To: ref.TimeRange.To,
		}, bearerEditor(bearer))
	return gatewayJSON("get finding", response, err)
}

func (client *Client) SearchSessions(ctx context.Context, projectID, bearer string, body gatewaycontract.SearchSessionsRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.SearchSessionsWithResponse(ctx,
		&gatewaycontract.SearchSessionsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("search sessions", response, err)
}

func (client *Client) GetSession(ctx context.Context, projectID, bearer string, ref gatewaycontract.SourceObjectRef) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	sourceInstance := ""
	if ref.SourceInstance != nil {
		sourceInstance = *ref.SourceInstance
	}
	response, err := client.api.GetSessionWithResponse(ctx, ref.SourceCode, ref.ExternalId, &gatewaycontract.GetSessionParams{
		XProjectID: projectID, SourceInstance: sourceInstance, From: ref.TimeRange.From, To: ref.TimeRange.To,
	}, bearerEditor(bearer))
	return gatewayJSON("get session", response, err)
}

func (client *Client) SearchEndpoints(ctx context.Context, projectID, bearer string, body gatewaycontract.SearchEndpointsRequest) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.SearchEndpointsWithResponse(ctx,
		&gatewaycontract.SearchEndpointsParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("search endpoints", response, err)
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

func gatewayJSON(operation string, response gatewayResponse, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, fmt.Errorf("Gateway %s: %w", operation, err)
	}
	status := response.StatusCode()
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		var envelope gatewaycontract.ErrorEnvelope
		if json.Unmarshal(response.GetBody(), &envelope) != nil {
			return nil, &HTTPError{Status: status, Code: "gateway_error", Message: "Gateway rejected the request"}
		}
		return nil, &HTTPError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	if !json.Valid(response.GetBody()) {
		return nil, fmt.Errorf("decode Gateway %s response: expected application/json", operation)
	}
	return json.RawMessage(response.GetBody()), nil
}
