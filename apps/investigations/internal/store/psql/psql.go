package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// ErrAmbiguousTenant — у субъекта роли в нескольких проектах, а запрос не
// указывает, в каком он действует.
var ErrAmbiguousTenant = errors.New("subject has role bindings in multiple projects")

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, uri string, maxConns int, connMaxLifetime time.Duration) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(uri)
	if err != nil {
		return nil, fmt.Errorf("parse database uri: %w", err)
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MaxConnLifetime = connMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Ping(ctx context.Context) error { return d.pool.Ping(ctx) }

func (d *DB) Close() { d.pool.Close() }

func (d *DB) RoleBindings(ctx context.Context, subjectID string) (model.SubjectRoles, error) {
	rows, err := d.pool.Query(ctx, `
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
	return model.SubjectRoles{}, ErrAmbiguousTenant
}

func (d *DB) ListSources(ctx context.Context) ([]model.Source, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT code, kind, title, secret_ref, is_enabled
		  FROM sources
		 ORDER BY code
	`)
	if err != nil {
		return nil, fmt.Errorf("query sources: %w", err)
	}
	defer rows.Close()

	var out []model.Source
	for rows.Next() {
		var item model.Source
		if err := rows.Scan(&item.Code, &item.Kind, &item.Title, &item.SecretRef, &item.IsEnabled); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) ListEntityTypes(ctx context.Context) ([]model.EntityType, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT code, title
		  FROM entity_types
		 ORDER BY code
	`)
	if err != nil {
		return nil, fmt.Errorf("query entity types: %w", err)
	}
	defer rows.Close()

	var out []model.EntityType
	for rows.Next() {
		var item model.EntityType
		if err := rows.Scan(&item.Code, &item.Title); err != nil {
			return nil, fmt.Errorf("scan entity type: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) ListRelationTypes(ctx context.Context) ([]model.RelationType, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT code, title, source_kind, target_kind, directed
		  FROM relation_types
		 ORDER BY code
	`)
	if err != nil {
		return nil, fmt.Errorf("query relation types: %w", err)
	}
	defer rows.Close()

	var out []model.RelationType
	for rows.Next() {
		var item model.RelationType
		if err := rows.Scan(&item.Code, &item.Title, &item.SourceKind, &item.TargetKind, &item.Directed); err != nil {
			return nil, fmt.Errorf("scan relation type: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
