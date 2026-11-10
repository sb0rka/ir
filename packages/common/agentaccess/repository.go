package agentaccess

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) AgentSecret(ctx context.Context, project string) (Record, error) {
	var record Record
	err := r.pool.QueryRow(ctx, `SELECT project_id,secret_id::text,version_no,digest_alg,digest,prefix,postfix,created_at,updated_at
		FROM ir_agent_secret_digests WHERE project_id=$1`, project).Scan(&record.ProjectID, &record.SecretID, &record.VersionNo,
		&record.DigestAlg, &record.Digest, &record.Prefix, &record.Postfix, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read agent access: %w", err)
	}
	return record, nil
}

func (r *Repository) Verify(ctx context.Context, project, raw string) (Record, error) {
	if !Valid(raw) {
		return Record{}, ErrUnauthorized
	}
	record, err := r.AgentSecret(ctx, project)
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrUnauthorized
	}
	if err != nil {
		return Record{}, err
	}
	if record.DigestAlg != "sha256" || !Matches(raw, record.Digest) {
		return Record{}, ErrUnauthorized
	}
	return record, nil
}

func (r *Repository) Configuration(ctx context.Context, project string) (Configuration, error) {
	var record Configuration
	err := r.pool.QueryRow(ctx, `SELECT project_id,COALESCE(secret_id::text,''),version_no,encrypted_config,updated_at
		FROM ir_gateway_configurations WHERE project_id=$1`, project).Scan(&record.ProjectID, &record.SecretID, &record.VersionNo, &record.EncryptedConfig, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrConfigurationMissing
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("read gateway configuration: %w", err)
	}
	return record, nil
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func putConfiguration(ctx context.Context, db executor, record Configuration) error {
	_, err := db.Exec(ctx, `INSERT INTO ir_gateway_configurations(project_id,secret_id,version_no,encrypted_config,updated_at)
		VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5)
		ON CONFLICT(project_id) DO UPDATE SET secret_id=EXCLUDED.secret_id,version_no=EXCLUDED.version_no,
		 encrypted_config=EXCLUDED.encrypted_config,updated_at=EXCLUDED.updated_at
		WHERE EXCLUDED.updated_at >= ir_gateway_configurations.updated_at
		 AND (ir_gateway_configurations.secret_id IS DISTINCT FROM EXCLUDED.secret_id
		      OR EXCLUDED.version_no >= ir_gateway_configurations.version_no)`,
		record.ProjectID, record.SecretID, record.VersionNo, record.EncryptedConfig, record.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save gateway configuration: %w", err)
	}
	return nil
}

func (r *Repository) PutConfiguration(ctx context.Context, record Configuration) error {
	return putConfiguration(ctx, r.pool, record)
}

func (r *Repository) SaveAgent(ctx context.Context, record Record, configuration Configuration, expectedVersion int32) error {
	if configuration.ProjectID != record.ProjectID || expectedVersion < 0 {
		return ErrConflict
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var result pgconn.CommandTag
	if expectedVersion == 0 {
		result, err = tx.Exec(ctx, `INSERT INTO ir_agent_secret_digests(project_id,secret_id,version_no,digest_alg,digest,prefix,postfix)
			VALUES($1,$2::uuid,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, record.ProjectID, record.SecretID, record.VersionNo,
			record.DigestAlg, record.Digest, record.Prefix, record.Postfix)
	} else {
		result, err = tx.Exec(ctx, `UPDATE ir_agent_secret_digests SET secret_id=$2::uuid,version_no=$3,digest_alg=$4,
			digest=$5,prefix=$6,postfix=$7,updated_at=now() WHERE project_id=$1 AND version_no=$8`, record.ProjectID,
			record.SecretID, record.VersionNo, record.DigestAlg, record.Digest, record.Prefix, record.Postfix, expectedVersion)
	}
	if err != nil {
		return fmt.Errorf("save agent access: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	if err = putConfiguration(ctx, tx, configuration); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Lock(ctx context.Context, project string) (func(), error) {
	// A dedicated session keeps the lock across upstream calls without exhausting a one-connection application pool.
	conn, err := pgx.ConnectConfig(ctx, r.pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("connect agent access lock: %w", err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, "ir-agent-access:"+project); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
		return nil, fmt.Errorf("lock agent access: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = conn.Exec(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "ir-agent-access:"+project)
			_ = conn.Close(cleanup)
		})
	}, nil
}
