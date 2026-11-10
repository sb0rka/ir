package gatewayclient

import (
	"context"
	"encoding/json"

	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
)

func (client *Client) ListEventFields(ctx context.Context, projectID, bearer string) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.ListEventFieldsWithResponse(ctx, &gatewaycontract.ListEventFieldsParams{XProjectID: projectID}, bearerEditor(bearer))
	return gatewayJSON("list event fields", response, err)
}
