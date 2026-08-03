package psql

import (
	"context"
	"fmt"
	"time"

	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// Пул, Ping и Close живут в core: подключение одинаково во всех сервисах.
type DB struct {
	*corestore.Pool
}

func New(_ context.Context, uri string, maxConns int, connMaxLifetime time.Duration) (*DB, error) {
	pool, err := corestore.NewPool(uri, maxConns, connMaxLifetime)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) RoleBindings(ctx context.Context, subjectID, projectID string) (model.SubjectRoles, error) {
	rows, err := d.Pgx().Query(ctx, `
		SELECT role
		  FROM role_bindings
		 WHERE subject_id = $1
		   AND project_id = $2
		 ORDER BY role
	`, subjectID, projectID)
	if err != nil {
		return model.SubjectRoles{}, fmt.Errorf("query role bindings: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return model.SubjectRoles{}, fmt.Errorf("scan role binding: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return model.SubjectRoles{}, fmt.Errorf("iterate role bindings: %w", err)
	}

	return model.SubjectRoles{ProjectID: projectID, Roles: roles}, nil
}
