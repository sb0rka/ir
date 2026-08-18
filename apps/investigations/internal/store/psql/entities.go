package psql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

func scanEntity(row pgx.Row) (model.Entity, error) {
	var out model.Entity
	err := row.Scan(&out.ID, &out.TypeCode, &out.CanonicalKey, &out.DisplayName, &out.Metadata, &out.FirstSeen, &out.LastSeen, &out.AddedVia, &out.AddedAt)
	return out, err
}

func (d *DB) entitySources(ctx context.Context, projectID string, entityIDs []string) (map[string][]model.EntitySource, error) {
	out := make(map[string][]model.EntitySource, len(entityIDs))
	if len(entityIDs) == 0 {
		return out, nil
	}
	rows, err := d.Pgx().Query(ctx, `SELECT entity_id::text,source_code,source_entity_id,source_ref,fetched_at FROM entity_sources WHERE project_id=$1 AND entity_id::text=ANY($2::text[]) ORDER BY source_code,source_entity_id`, projectID, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("list entity sources: %w", mapConstraint(err))
	}
	defer rows.Close()
	for rows.Next() {
		var entityID string
		var source model.EntitySource
		if err := rows.Scan(&entityID, &source.SourceCode, &source.SourceEntityID, &source.SourceRef, &source.FetchedAt); err != nil {
			return nil, err
		}
		out[entityID] = append(out[entityID], source)
	}
	return out, rows.Err()
}

func (d *DB) InvestigationEntities(ctx context.Context, projectID, investigationID string, filter model.EntityFilter) ([]model.Entity, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, store.ErrInvestigationNotFound
	}
	var cursorTime, cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	rows, err := d.Pgx().Query(ctx, `SELECT e.id::text,e.type_code,e.canonical_key,e.display_name,e.metadata,e.first_seen,e.last_seen,ie.added_via,ie.added_at FROM investigation_entities ie JOIN entities e ON e.id=ie.entity_id WHERE ie.investigation_id=$1::uuid AND ie.project_id=$2 AND ($3::text IS NULL OR e.type_code=$3) AND ($4::text IS NULL OR e.canonical_key ILIKE '%'||$4||'%' OR e.display_name ILIKE '%'||$4||'%') AND ($5::timestamptz IS NULL OR (ie.added_at,e.id)<($5,$6::uuid)) ORDER BY ie.added_at DESC,e.id DESC LIMIT $7`, investigationID, projectID, filter.TypeCode, filter.Q, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.Entity
	for rows.Next() {
		item, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	ids := make([]string, 0, len(out))
	for _, item := range out {
		ids = append(ids, item.ID)
	}
	sources, err := d.entitySources(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Sources = sources[out[i].ID]
	}
	return out, nil
}

func (d *DB) CreateEntity(ctx context.Context, input model.EntityNew) (model.Entity, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.Entity{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, input.ProjectID, input.InvestigationID)
	if err != nil {
		return model.Entity{}, err
	}
	if !exists {
		return model.Entity{}, store.ErrInvestigationNotFound
	}
	metadata := "{}"
	if len(input.Metadata) > 0 {
		metadata = string(input.Metadata)
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO entities(project_id,type_code,canonical_key,display_name,metadata) VALUES($1,$2,$3,$4,$5::jsonb) ON CONFLICT(project_id,type_code,canonical_key) DO UPDATE SET display_name=COALESCE(EXCLUDED.display_name,entities.display_name),metadata=EXCLUDED.metadata RETURNING id::text`, input.ProjectID, input.TypeCode, input.CanonicalKey, input.DisplayName, metadata).Scan(&id)
	if err != nil {
		return model.Entity{}, fmt.Errorf("create entity: %w", mapConstraint(err))
	}
	_, err = tx.Exec(ctx, `INSERT INTO investigation_entities(investigation_id,entity_id,project_id,added_via) VALUES($1::uuid,$2::uuid,$3,'analyst') ON CONFLICT DO NOTHING`, input.InvestigationID, id, input.ProjectID)
	if err != nil {
		return model.Entity{}, mapConstraint(err)
	}
	if _, _, err = upsertNodeTx(ctx, tx, input.InvestigationID, "entity", &id, nil, "analyst", nil); err != nil {
		return model.Entity{}, err
	}
	out, err := scanEntity(tx.QueryRow(ctx, `SELECT e.id::text,e.type_code,e.canonical_key,e.display_name,e.metadata,e.first_seen,e.last_seen,ie.added_via,ie.added_at FROM entities e JOIN investigation_entities ie ON ie.entity_id=e.id WHERE e.id=$1::uuid AND e.project_id=$2 AND ie.investigation_id=$3::uuid`, id, input.ProjectID, input.InvestigationID))
	if err != nil {
		return model.Entity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Entity{}, err
	}
	return out, nil
}

func (d *DB) GetEntityCard(ctx context.Context, projectID, entityID string) (model.EntityCard, error) {
	var out model.EntityCard
	err := d.Pgx().QueryRow(ctx, `SELECT id::text,type_code,canonical_key,display_name,metadata,first_seen,last_seen,NULL::text,created_at FROM entities WHERE id=$1::uuid AND project_id=$2`, entityID, projectID).Scan(&out.Entity.ID, &out.Entity.TypeCode, &out.Entity.CanonicalKey, &out.Entity.DisplayName, &out.Entity.Metadata, &out.Entity.FirstSeen, &out.Entity.LastSeen, &out.Entity.AddedVia, &out.Entity.AddedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EntityCard{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.EntityCard{}, fmt.Errorf("get entity: %w", mapConstraint(err))
	}
	sources, err := d.entitySources(ctx, projectID, []string{entityID})
	if err != nil {
		return model.EntityCard{}, err
	}
	out.Entity.Sources = sources[entityID]
	err = d.Pgx().QueryRow(ctx, `SELECT count(DISTINCT event_id)::int FROM event_entity_relations WHERE entity_id=$1::uuid AND project_id=$2`, entityID, projectID).Scan(&out.EventsCount)
	if err != nil {
		return model.EntityCard{}, err
	}
	rows, err := d.Pgx().Query(ctx, `SELECT i.id::text,i.title,count(DISTINCT ie.event_id)::int FROM investigation_entities ient JOIN investigations i ON i.id=ient.investigation_id LEFT JOIN investigation_events ie ON ie.investigation_id=i.id LEFT JOIN event_entity_relations r ON r.event_id=ie.event_id AND r.entity_id=ient.entity_id WHERE ient.entity_id=$1::uuid AND ient.project_id=$2 GROUP BY i.id,i.title ORDER BY i.created_at DESC`, entityID, projectID)
	if err != nil {
		return model.EntityCard{}, err
	}
	for rows.Next() {
		var item model.EntityOccurrence
		if err := rows.Scan(&item.InvestigationID, &item.Title, &item.EventsCount); err != nil {
			rows.Close()
			return model.EntityCard{}, err
		}
		out.Occurrences = append(out.Occurrences, item)
	}
	rows.Close()
	rows, err = d.Pgx().Query(ctx, `SELECT DISTINCT other.entity_id::text,ent.display_name,e.relation_code FROM graph_nodes self JOIN edges e ON e.status='confirmed' AND (e.source_node_id=self.id OR e.target_node_id=self.id) JOIN graph_nodes other ON other.id=CASE WHEN e.source_node_id=self.id THEN e.target_node_id ELSE e.source_node_id END JOIN investigations i ON i.id=e.investigation_id JOIN entities ent ON ent.id=other.entity_id WHERE self.entity_id=$1::uuid AND other.entity_id IS NOT NULL AND i.project_id=$2 ORDER BY other.entity_id::text,e.relation_code`, entityID, projectID)
	if err != nil {
		return model.EntityCard{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var n model.EntityNeighbor
		if err := rows.Scan(&n.EntityID, &n.DisplayName, &n.RelationCode); err != nil {
			return model.EntityCard{}, err
		}
		out.Neighbors = append(out.Neighbors, n)
	}
	return out, rows.Err()
}

func (d *DB) DetachEntity(ctx context.Context, projectID, investigationID, entityID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, projectID, investigationID)
	if err != nil {
		return err
	}
	if !exists {
		return store.ErrInvestigationNotFound
	}
	var used bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_entity_relations r JOIN investigation_events ie ON ie.event_id=r.event_id WHERE r.entity_id=$1::uuid AND ie.investigation_id=$2::uuid AND ie.project_id=$3)`, entityID, investigationID, projectID).Scan(&used)
	if err != nil {
		return mapConstraint(err)
	}
	if used {
		return store.ErrConflict
	}
	tag, err := tx.Exec(ctx, `DELETE FROM investigation_entities WHERE investigation_id=$1::uuid AND entity_id=$2::uuid AND project_id=$3`, investigationID, entityID, projectID)
	if err != nil {
		return mapConstraint(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrRecordNotFound
	}
	return tx.Commit(ctx)
}
