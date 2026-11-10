package psql

import (
	"context"

	"github.com/sb0rka/ir/packages/common/agentaccess"
)

func (d *DB) AgentSecret(ctx context.Context, project string) (agentaccess.Record, error) {
	return agentaccess.NewRepository(d.Pgx()).AgentSecret(ctx, project)
}
func (d *DB) Verify(ctx context.Context, project, raw string) (agentaccess.Record, error) {
	return agentaccess.NewRepository(d.Pgx()).Verify(ctx, project, raw)
}
func (d *DB) Configuration(ctx context.Context, project string) (agentaccess.Configuration, error) {
	return agentaccess.NewRepository(d.Pgx()).Configuration(ctx, project)
}
func (d *DB) PutConfiguration(ctx context.Context, configuration agentaccess.Configuration) error {
	return agentaccess.NewRepository(d.Pgx()).PutConfiguration(ctx, configuration)
}
func (d *DB) SaveAgent(ctx context.Context, record agentaccess.Record, configuration agentaccess.Configuration, expectedVersion int32) error {
	return agentaccess.NewRepository(d.Pgx()).SaveAgent(ctx, record, configuration, expectedVersion)
}
func (d *DB) Lock(ctx context.Context, project string) (func(), error) {
	return agentaccess.NewRepository(d.Pgx()).Lock(ctx, project)
}
