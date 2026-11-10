package store

import (
	"context"

	"github.com/sb0rka/ir/packages/common/agentaccess"
)

type AgentAccessDatabase interface {
	AgentSecret(context.Context, string) (agentaccess.Record, error)
	Verify(context.Context, string, string) (agentaccess.Record, error)
	Configuration(context.Context, string) (agentaccess.Configuration, error)
	PutConfiguration(context.Context, agentaccess.Configuration) error
	SaveAgent(context.Context, agentaccess.Record, agentaccess.Configuration, int32) error
	Lock(context.Context, string) (func(), error)
}
