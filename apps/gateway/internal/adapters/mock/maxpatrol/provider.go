package maxpatrol

import (
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/entitymock"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/eventmock"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const SourceCode = "maxpatrol-siem"

func NewMock(value scenario.Scenario) registry.Provider {
	events := eventmock.New(value, "MaxPatrol", SourceCode, func(event scenario.Event, attributes map[string]any) {
		attributes["generator"] = "correlationengine"
		attributes["normalized"] = true
		if strings.Contains(strings.ToLower(event.EventClass), "correlation") {
			attributes["correlation_name"] = "impacket_smbexec"
		}
	})
	entities := entitymock.New(value, "MaxPatrol", SourceCode, nil)
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "MaxPatrol SIEM",
			Kind:         "siem",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEvents, domain.CapabilityEntityLookup},
		},
		Events:       events,
		EntityLookup: entities,
	}
}
