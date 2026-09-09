package gatewayclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
)

func (client *Client) CreateEvidenceExport(ctx context.Context, projectID, bearer string, body gatewaycontract.EvidenceReference) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	response, err := client.api.CreateEvidenceExportWithResponse(ctx, &gatewaycontract.CreateEvidenceExportParams{XProjectID: projectID}, body, bearerEditor(bearer))
	return gatewayJSON("create evidence export", response, err)
}

func (client *Client) GetEvidenceExport(ctx context.Context, projectID, bearer, exportID string) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(exportID)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments: export_id must be a UUID")
	}
	response, err := client.api.GetEvidenceExportWithResponse(ctx, id, &gatewaycontract.GetEvidenceExportParams{XProjectID: projectID}, bearerEditor(bearer))
	return gatewayJSON("get evidence export", response, err)
}

type EvidenceChunk struct {
	ExportID   string `json:"export_id"`
	Encoding   string `json:"encoding"`
	Content    string `json:"content"`
	Size       int    `json:"size"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	EOF        bool   `json:"eof"`
}

// Base64 preserves every byte, including UTF-8 characters split across chunks.
func (client *Client) ReadEvidenceContent(ctx context.Context, projectID, bearer, exportID string, offset int64, limit int) (json.RawMessage, error) {
	if err := client.ready(); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(exportID)
	if err != nil {
		return nil, fmt.Errorf("invalid arguments: export_id must be a UUID")
	}
	if limit == 0 {
		limit = 16 << 10
	}
	if offset < 0 || offset > (1<<63-1)-int64(limit) || limit < 1 || limit > 64<<10 {
		return nil, fmt.Errorf("invalid arguments: offset must be nonnegative and limit between 1 and 65536")
	}
	response, err := client.api.GetEvidenceContent(ctx, id, &gatewaycontract.GetEvidenceContentParams{XProjectID: projectID, Offset: &offset, Limit: &limit}, bearerEditor(bearer))
	if err != nil {
		return nil, fmt.Errorf("Gateway read evidence: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("Gateway read evidence: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		var envelope gatewaycontract.ErrorEnvelope
		if json.Unmarshal(raw, &envelope) != nil {
			return nil, &HTTPError{Status: response.StatusCode, Code: "gateway_error", Message: "Gateway rejected the request"}
		}
		return nil, &HTTPError{Status: response.StatusCode, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	eof, err := strconv.ParseBool(response.Header.Get("X-Evidence-EOF"))
	if err != nil || len(raw) > limit || (!eof && len(raw) == 0) {
		return nil, fmt.Errorf("Gateway returned an invalid evidence chunk")
	}
	return json.Marshal(EvidenceChunk{ExportID: id.String(), Encoding: "base64", Content: base64.StdEncoding.EncodeToString(raw), Size: len(raw), Offset: offset, NextOffset: offset + int64(len(raw)), EOF: eof})
}
