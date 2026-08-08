package mock

import (
	"fmt"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/sandbox"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Options struct {
	EventCount    int
	EndpointCount int
	HistoryDays   int
}

type Stats struct {
	EventCount    int
	EndpointCount int
}

func NewRegistry(options Options) (*registry.Registry, Stats, error) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("load scenario: %w", err)
	}
	value, err = scenario.Expand(value, scenario.GenerateOptions{
		EventCount:    options.EventCount,
		EndpointCount: options.EndpointCount,
		HistoryDays:   options.HistoryDays,
	})
	if err != nil {
		return nil, Stats{}, fmt.Errorf("expand scenario: %w", err)
	}
	providers, err := registry.New(
		maxpatrol.NewMock(value),
		sandbox.NewMock(),
	)
	if err != nil {
		return nil, Stats{}, err
	}
	return providers, Stats{EventCount: len(value.EventsForSource("MaxPatrol")), EndpointCount: options.EndpointCount}, nil
}
