package adapters

import (
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/edr"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/fusion"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/ptnad"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/sandbox"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func NewMockRegistry(value scenario.Scenario) (*registry.Registry, error) {
	return registry.New(
		maxpatrol.NewMock(value),
		ptnad.NewMock(value),
		edr.NewMock(value),
		sandbox.NewMock(),
		fusion.NewMock(),
	)
}
