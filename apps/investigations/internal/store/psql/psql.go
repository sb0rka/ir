package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

type DB struct{ *corestore.Pool }

func New(_ context.Context, uri string, maxConns int, connMaxLifetime time.Duration) (*DB, error) {
	pool, err := corestore.NewPool(uri, maxConns, connMaxLifetime)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) RoleBindings(ctx context.Context, subjectID, projectID string) (model.SubjectRoles, error) {
	rows, err := d.Pgx().Query(ctx, `SELECT role FROM role_bindings WHERE subject_id=$1 AND project_id=$2 ORDER BY role`, subjectID, projectID)
	if err != nil {
		return model.SubjectRoles{}, fmt.Errorf("query role bindings: %w", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return model.SubjectRoles{}, err
		}
		roles = append(roles, role)
	}
	return model.SubjectRoles{ProjectID: projectID, Roles: roles}, rows.Err()
}

const (
	pgForeignKeyViolation       = "23503"
	pgCheckViolation            = "23514"
	pgInvalidTextRepresentation = "22P02"
	pgUniqueViolation           = "23505"
)

func investigationExistsTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string) (bool, error) {
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM investigations WHERE id=$1::uuid AND project_id=$2`, investigationID, projectID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgInvalidTextRepresentation {
			return false, nil
		}
		return false, fmt.Errorf("check investigation: %w", err)
	}
	return true, nil
}

func (d *DB) InvestigationExists(ctx context.Context, projectID, investigationID string) (bool, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return investigationExistsTx(ctx, tx, projectID, investigationID)
}

func mapConstraint(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case pgForeignKeyViolation:
		return store.ErrUnknownReference
	case pgCheckViolation, pgInvalidTextRepresentation, pgUniqueViolation:
		return store.ErrInvalidValue
	default:
		return err
	}
}
