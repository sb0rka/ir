package psql

import (
	"context"
	"fmt"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

func (d *DB) Reference(ctx context.Context) (model.Reference, error) {
	var out model.Reference
	rows, err := d.Pgx().Query(ctx, `SELECT code,title,category FROM entity_types ORDER BY code`)
	if err != nil {
		return out, fmt.Errorf("entity types: %w", err)
	}
	for rows.Next() {
		var item model.EntityType
		if err := rows.Scan(&item.Code, &item.Title, &item.Category); err != nil {
			rows.Close()
			return out, err
		}
		out.EntityTypes = append(out.EntityTypes, item)
	}
	rows.Close()
	rows, err = d.Pgx().Query(ctx, `SELECT code,title,source_kind,target_kind,directed FROM relation_types ORDER BY code`)
	if err != nil {
		return out, fmt.Errorf("relation types: %w", err)
	}
	for rows.Next() {
		var item model.RelationType
		if err := rows.Scan(&item.Code, &item.Title, &item.SourceKind, &item.TargetKind, &item.Directed); err != nil {
			rows.Close()
			return out, err
		}
		out.RelationTypes = append(out.RelationTypes, item)
	}
	rows.Close()
	rows, err = d.Pgx().Query(ctx, `SELECT code,kind,title FROM sources WHERE is_enabled ORDER BY code`)
	if err != nil {
		return out, fmt.Errorf("sources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item model.Source
		if err := rows.Scan(&item.Code, &item.Kind, &item.Title); err != nil {
			return out, err
		}
		out.Sources = append(out.Sources, item)
	}
	return out, rows.Err()
}
