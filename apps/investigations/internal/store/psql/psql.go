package psql

import (
	"context"
	"fmt"
	"time"

	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

// DB — пул платформы плюс запросы расследований. Сам пул, Ping и Close живут
// в core: подключение к Postgres одинаково во всех сервисах, отличаются только
// запросы поверх него.
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

func (d *DB) RoleBindings(ctx context.Context, subjectID string) (model.SubjectRoles, error) {
	rows, err := d.Pgx().Query(ctx, `
		SELECT project_id, role
		  FROM role_bindings
		 WHERE subject_id = $1
		 ORDER BY role
	`, subjectID)
	if err != nil {
		return model.SubjectRoles{}, fmt.Errorf("query role bindings: %w", err)
	}
	defer rows.Close()

	// Роли собираются по проектам, а не сваливаются в один список: субъект
	// штатно имеет биндинги в нескольких тенантах, и склейка дала бы права
	// одного проекта в контексте другого.
	byProject := make(map[string][]string)
	for rows.Next() {
		var projectID, role string
		if err := rows.Scan(&projectID, &role); err != nil {
			return model.SubjectRoles{}, fmt.Errorf("scan role binding: %w", err)
		}
		byProject[projectID] = append(byProject[projectID], role)
	}
	if err := rows.Err(); err != nil {
		return model.SubjectRoles{}, fmt.Errorf("iterate role bindings: %w", err)
	}

	switch len(byProject) {
	case 0:
		return model.SubjectRoles{}, nil
	case 1:
		for projectID, roles := range byProject {
			return model.SubjectRoles{ProjectID: projectID, Roles: roles}, nil
		}
	}
	// Выбирать тенант молча нельзя: запрос не говорит, какой из них имелся
	// в виду. Появится мультитенантный клиент — тенант станет явным входом.
	return model.SubjectRoles{}, store.ErrAmbiguousTenant
}
