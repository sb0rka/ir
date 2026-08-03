package edr

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

const SourceCode = "maxpatrol-edr"

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type mock struct {
	endpoints []domain.Endpoint
}

func NewMock(value scenario.Scenario) registry.Provider {
	adapter := &mock{endpoints: buildEndpoints(value)}
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "MaxPatrol EDR",
			Kind:         "edr",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEndpoints, domain.CapabilityResponseCatalog},
		},
		Endpoints:       adapter,
		ResponseCatalog: adapter,
	}
}

func (adapter *mock) SearchEndpoints(ctx context.Context, request capability.SearchEndpointsRequest) (capability.EndpointPage, error) {
	if err := ctx.Err(); err != nil {
		return capability.EndpointPage{}, err
	}
	offset := 0
	if request.Cursor != "" {
		value, err := strconv.Atoi(request.Cursor)
		if err != nil || value < 0 {
			return capability.EndpointPage{}, &domain.RequestError{Code: "invalid_cursor", Message: "endpoint cursor is invalid"}
		}
		offset = value
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	items := make([]domain.Endpoint, 0, len(adapter.endpoints))
	for _, endpoint := range adapter.endpoints {
		if query == "" || strings.Contains(strings.ToLower(endpoint.Hostname+" "+strings.Join(endpoint.IPAddresses, " ")), query) {
			items = append(items, endpoint)
		}
	}
	if offset > len(items) {
		return capability.EndpointPage{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is past the result set"}
	}
	limit := request.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	page := capability.EndpointPage{Items: items[offset:end], HasMore: end < len(items)}
	for index := range page.Items {
		page.Continuations = append(page.Continuations, strconv.Itoa(offset+index+1))
	}
	return page, nil
}

func (adapter *mock) ListResponseActions(ctx context.Context, externalID string) ([]domain.ResponseAction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, endpoint := range adapter.endpoints {
		if endpoint.ExternalID == externalID {
			return []domain.ResponseAction{
				{Code: "collect_diagnostics", Title: "Collect diagnostics", Enabled: true},
				{Code: "isolate_network", Title: "Isolate from network", Destructive: true, Enabled: true},
				{Code: "terminate_process", Title: "Terminate process", Destructive: true, Enabled: true},
				{Code: "delete_file", Title: "Delete file", Destructive: true, Enabled: true},
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: endpoint %q", domain.ErrNotFound, externalID)
}

func buildEndpoints(value scenario.Scenario) []domain.Endpoint {
	items := make([]domain.Endpoint, 0, 2)
	for _, node := range value.NodesForSystem("MaxPatrol") {
		if node.Data.Kind != "host" {
			continue
		}
		hostname := strings.TrimSuffix(node.Data.Label, "...")
		externalID := "agent-" + strings.Split(hostname, ".")[0]
		ip := "10.125.10.44"
		status := "online"
		if strings.HasPrefix(hostname, "euskova") {
			ip = "10.125.10.62"
			status = "offline"
		}
		items = append(items, domain.Endpoint{
			ID:          domain.StableID("endpoint", SourceCode, externalID),
			ExternalID:  externalID,
			Hostname:    hostname,
			IPAddresses: []string{ip},
			Status:      status,
			Attributes: map[string]any{
				"agent_server": "edr.qa-ptlab.ru",
				"modules":      []string{"telemetry", "response"},
			},
			Provenance: domain.Provenance{Source: SourceCode, ExternalID: externalID, FetchedAt: fetchedAt},
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Hostname < items[j].Hostname })
	return items
}
