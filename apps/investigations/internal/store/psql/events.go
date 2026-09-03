package psql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

func (d *DB) InvestigationEvents(ctx context.Context, projectID, investigationID string, filter model.EventFilter) ([]model.EventSummary, error) {
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
	rows, err := d.Pgx().Query(ctx, `
		SELECT e.id::text,e.source_code,e.source_event_id,e.source_ref,e.title,
		       e.event_type,e.occurred_at,e.ingested_at,ie.attached_at,ie.attached_by,ie.reason,ie.is_seed,e.normalized_data
		  FROM investigation_events ie JOIN events e ON e.id=ie.event_id
		 WHERE ie.investigation_id=$1::uuid AND ie.project_id=$2
		   AND ($3::text IS NULL OR e.event_type=$3)
		   AND ($4::text IS NULL OR e.source_code=$4)
		   AND ($5::uuid IS NULL OR EXISTS (SELECT 1 FROM event_entity_relations r WHERE r.event_id=e.id AND r.entity_id=$5::uuid AND r.project_id=$2))
		   AND ($6::timestamptz IS NULL OR e.occurred_at >= $6)
		   AND ($7::timestamptz IS NULL OR e.occurred_at < $7)
		   AND ($8::text IS NULL OR e.title ILIKE '%'||$8||'%' OR e.normalized_data::text ILIKE '%'||$8||'%')
		   AND ($9::timestamptz IS NULL OR (e.occurred_at,e.id) > ($9,$10::uuid))
		 ORDER BY e.occurred_at,e.id LIMIT $11`, investigationID, projectID, filter.EventType, filter.SourceCode,
		filter.EntityID, filter.From, filter.To, filter.Q, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("query investigation events: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.EventSummary
	for rows.Next() {
		var item model.EventSummary
		if err := rows.Scan(&item.ID, &item.SourceCode, &item.SourceEventID, &item.SourceRef, &item.Title, &item.EventType, &item.OccurredAt, &item.IngestedAt, &item.AttachedAt, &item.AttachedBy, &item.Reason, &item.IsSeed, &item.NormalizedData); err != nil {
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
	relations, err := d.eventEntities(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Entities = relations[out[i].ID]
	}
	return out, nil
}

func (d *DB) eventEntities(ctx context.Context, projectID string, eventIDs []string) (map[string][]model.EventEntity, error) {
	out := make(map[string][]model.EventEntity, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}
	rows, err := d.Pgx().Query(ctx, `SELECT event_id::text,entity_id::text,relation_code FROM event_entity_relations WHERE project_id=$1 AND event_id::text=ANY($2::text[]) ORDER BY event_id,entity_id,relation_code`, projectID, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("list event entities: %w", mapConstraint(err))
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		var rel model.EventEntity
		if err := rows.Scan(&eventID, &rel.EntityID, &rel.RelationCode); err != nil {
			return nil, err
		}
		out[eventID] = append(out[eventID], rel)
	}
	return out, rows.Err()
}

func (d *DB) GetEvent(ctx context.Context, projectID, eventID string) (model.Event, error) {
	var out model.Event
	err := d.Pgx().QueryRow(ctx, `SELECT id::text,source_code,source_event_id,source_ref,title,event_type,occurred_at,ingested_at,normalized_data FROM events WHERE id=$1::uuid AND project_id=$2`, eventID, projectID).Scan(&out.ID, &out.SourceCode, &out.SourceEventID, &out.SourceRef, &out.Title, &out.EventType, &out.OccurredAt, &out.IngestedAt, &out.NormalizedData)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Event{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.Event{}, fmt.Errorf("get event: %w", mapConstraint(err))
	}
	rows, err := d.Pgx().Query(ctx, `SELECT ie.investigation_id::text
		FROM investigation_events ie JOIN investigations i
		  ON i.id=ie.investigation_id AND i.project_id=ie.project_id AND i.is_deleted=false
		WHERE ie.event_id=$1::uuid AND ie.project_id=$2 ORDER BY ie.investigation_id`, eventID, projectID)
	if err != nil {
		return model.Event{}, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return model.Event{}, err
		}
		out.InvestigationIDs = append(out.InvestigationIDs, id)
	}
	rows.Close()
	rows, err = d.Pgx().Query(ctx, `SELECT entity_id::text,relation_code FROM event_entity_relations WHERE event_id=$1::uuid AND project_id=$2 ORDER BY entity_id,relation_code`, eventID, projectID)
	if err != nil {
		return model.Event{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var rel model.EventEntity
		if err := rows.Scan(&rel.EntityID, &rel.RelationCode); err != nil {
			return model.Event{}, err
		}
		out.Entities = append(out.Entities, rel)
	}
	return out, rows.Err()
}

func (d *DB) DetachEvent(ctx context.Context, projectID, investigationID, eventID string) error {
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
	var confirmed bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM edge_evidence ee JOIN edges e ON e.id=ee.edge_id WHERE ee.investigation_id=$1::uuid AND ee.event_id=$2::uuid AND e.status='confirmed')`, investigationID, eventID).Scan(&confirmed)
	if err != nil {
		return fmt.Errorf("check event evidence: %w", mapConstraint(err))
	}
	if confirmed {
		return store.ErrConflict
	}
	tag, err := tx.Exec(ctx, `DELETE FROM investigation_events WHERE investigation_id=$1::uuid AND event_id=$2::uuid AND project_id=$3`, investigationID, eventID, projectID)
	if err != nil {
		return fmt.Errorf("detach event: %w", mapConstraint(err))
	}
	if tag.RowsAffected() == 0 {
		return store.ErrRecordNotFound
	}
	return tx.Commit(ctx)
}

func (d *DB) InvestigationTimelineBounds(ctx context.Context, projectID, investigationID string) (*time.Time, *time.Time, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, store.ErrInvestigationNotFound
	}
	var from, to *time.Time
	err = d.Pgx().QueryRow(ctx, `
		SELECT min(e.occurred_at), max(e.occurred_at)
		  FROM investigation_events ie
		  JOIN events e ON e.id=ie.event_id AND e.project_id=ie.project_id
		 WHERE ie.investigation_id=$1::uuid AND ie.project_id=$2`, investigationID, projectID).Scan(&from, &to)
	if err != nil {
		return nil, nil, fmt.Errorf("investigation timeline bounds: %w", mapConstraint(err))
	}
	return from, to, nil
}
