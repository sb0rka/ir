package ptnad

import (
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/entitymock"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/eventmock"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

const SourceCode = "pt-nad"

func NewMock(value scenario.Scenario) registry.Provider {
	events := eventmock.New(value, "PT NAD", SourceCode, enrich)
	entities := entitymock.New(value, "PT NAD", SourceCode, nil)
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "PT NAD",
			Kind:         "ndr",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEvents, domain.CapabilityEntityLookup},
		},
		Events:       events,
		EntityLookup: entities,
	}
}

func enrich(event scenario.Event, attributes map[string]any) {
	attributes["proto"] = "TCP"
	attributes["app_proto"] = "http"
	attributes["src"] = map[string]any{"ip": "10.125.10.44"}
	attributes["dst"] = map[string]any{"ip": "10.125.11.90", "port": 8000}
	attributes["bytes"] = map[string]any{"sent": 1420, "received": 317440, "total": 318860}
	attributes["alert"] = map[string]any{"description": event.Title, "level": strings.ToLower(event.Severity)}
	if strings.Contains(strings.ToLower(event.EventClass), "download") {
		attributes["http"] = map[string]any{"method": "GET", "status_code": 200}
		attributes["files"] = map[string]any{"extracted": true}
	}
}
