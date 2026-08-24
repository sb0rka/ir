package psql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const investigationSelect = `
	SELECT i.id::text, i.project_id, i.parent_id::text, i.title, i.description,
	       i.status, i.severity, i.verdict, i.verdict_reason, i.confidence,
	       i.origin, i.origin_ref, i.version, i.created_at, i.updated_at, i.closed_at,
	       COALESCE((SELECT array_agg(w.workspace_id::text ORDER BY w.workspace_id)
	                   FROM investigation_som_workspaces w WHERE w.investigation_id=i.id), '{}'),
	       (SELECT count(*)::int FROM investigations c WHERE c.parent_id=i.id),
	       (SELECT count(*)::int FROM investigation_findings f WHERE f.investigation_id=i.id),
	       (SELECT count(*)::int FROM investigation_sessions s WHERE s.investigation_id=i.id),
	       (SELECT count(*)::int FROM investigation_events ie WHERE ie.investigation_id=i.id),
	       (SELECT count(*)::int FROM investigation_entities ie WHERE ie.investigation_id=i.id),
	       (SELECT count(*)::int FROM edges e WHERE e.investigation_id=i.id AND e.status='proposed')
	  FROM investigations i`

func scanInvestigation(row pgx.Row) (model.Investigation, error) {
	var out model.Investigation
	err := row.Scan(&out.ID, &out.ProjectID, &out.ParentID, &out.Title, &out.Description,
		&out.Status, &out.Severity, &out.Verdict, &out.VerdictReason, &out.Confidence,
		&out.Origin, &out.OriginRef, &out.Version, &out.CreatedAt, &out.UpdatedAt, &out.ClosedAt,
		&out.WorkspaceIDs, &out.Counters.Children, &out.Counters.Findings, &out.Counters.Sessions, &out.Counters.Events,
		&out.Counters.Entities, &out.Counters.ProposedEdges)
	return out, err
}

func investigationTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string) (model.Investigation, error) {
	out, err := scanInvestigation(tx.QueryRow(ctx, investigationSelect+` WHERE i.id=$1::uuid AND i.project_id=$2`, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Investigation{}, store.ErrInvestigationNotFound
	}
	if err != nil {
		return model.Investigation{}, fmt.Errorf("scan investigation: %w", err)
	}
	return out, nil
}

func (d *DB) CreateInvestigation(ctx context.Context, inv model.InvestigationNew) (model.Investigation, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.Investigation{}, fmt.Errorf("begin investigation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO investigations (project_id,title,description,severity,parent_id,origin)
		VALUES ($1,$2,$3,$4,$5::uuid,'analyst') RETURNING id::text
	`, inv.ProjectID, inv.Title, inv.Description, inv.Severity, inv.ParentID).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return model.Investigation{}, store.ErrParentNotFound
		}
		return model.Investigation{}, fmt.Errorf("insert investigation: %w", mapConstraint(err))
	}
	for _, workspaceID := range inv.WorkspaceIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO investigation_som_workspaces (investigation_id,project_id,workspace_id) VALUES ($1::uuid,$2,$3::uuid) ON CONFLICT DO NOTHING`, id, inv.ProjectID, workspaceID); err != nil {
			return model.Investigation{}, fmt.Errorf("link workspace: %w", mapConstraint(err))
		}
	}
	out, err := investigationTx(ctx, tx, inv.ProjectID, id)
	if err != nil {
		return model.Investigation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Investigation{}, fmt.Errorf("commit investigation: %w", err)
	}
	return out, nil
}

func (d *DB) GetInvestigation(ctx context.Context, projectID, investigationID string) (model.Investigation, error) {
	out, err := scanInvestigation(d.Pgx().QueryRow(ctx, investigationSelect+` WHERE i.id=$1::uuid AND i.project_id=$2`, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Investigation{}, store.ErrInvestigationNotFound
	}
	if err != nil {
		return model.Investigation{}, fmt.Errorf("get investigation: %w", err)
	}
	return out, nil
}

func (d *DB) ListInvestigations(ctx context.Context, projectID string, filter model.InvestigationFilter) ([]model.Investigation, error) {
	var parentID any
	if filter.ParentID != nil {
		parentID = *filter.ParentID
	}
	var cursorTime any
	var cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	rows, err := d.Pgx().Query(ctx, investigationSelect+`
	 WHERE i.project_id=$1
	   AND (($2::boolean AND i.parent_id IS NULL) OR ($3::uuid IS NOT NULL AND i.parent_id=$3::uuid) OR (NOT $2::boolean AND $3::uuid IS NULL))
	   AND ($4::text IS NULL OR i.status=$4)
	   AND ($5::text IS NULL OR i.severity=$5)
	   AND ($6::text IS NULL OR i.title ILIKE '%'||$6||'%')
	   AND ($7::timestamptz IS NULL OR (i.created_at,i.id) < ($7,$8::uuid))
	 ORDER BY i.created_at DESC,i.id DESC LIMIT $9`, projectID, filter.RootsOnly, parentID,
		filter.Status, filter.Severity, filter.Q, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list investigations: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.Investigation
	for rows.Next() {
		item, err := scanInvestigation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
