package psql

import (
	"context"
	"fmt"
	"time"

	corestore "github.com/sb0rka/sb0rka/packages/core/store"
)

type DB struct {
	*corestore.Pool
}

func New(uri string, maxConns int, connMaxLifetime time.Duration) (*DB, error) {
	pool, err := corestore.NewPool(uri, maxConns, connMaxLifetime)
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) HasRoleBinding(ctx context.Context, subjectID, projectID string) (bool, error) {
	var exists bool
	err := db.Pgx().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM role_bindings
			 WHERE subject_id = $1
			   AND project_id = $2
		)
	`, subjectID, projectID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("query role binding: %w", err)
	}
	return exists, nil
}
