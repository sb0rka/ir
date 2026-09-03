package psql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const sessionColumns = `
	s.id::text,s.project_id,s.source_code,s.source_instance,s.record_type,s.external_id,
	s.time_from,s.time_to,s.title,s.severity,s.started_at,s.ended_at,
	s.source_ref,s.fetched_at,s.normalized_snapshot,s.provenance,s.context_status,s.context_errors,
	s.created_at,s.updated_at`

func scanSession(row pgx.Row) (model.NetworkSession, error) {
	var out model.NetworkSession
	err := row.Scan(&out.ID, &out.ProjectID, &out.Ref.SourceCode, &out.Ref.SourceInstance,
		&out.Ref.RecordType, &out.Ref.ExternalID, &out.Ref.TimeRange.From, &out.Ref.TimeRange.To,
		&out.Title, &out.Severity, &out.StartedAt, &out.EndedAt, &out.SourceRef, &out.FetchedAt, &out.Normalized,
		&out.Provenance, &out.ContextStatus, &out.ContextErrors, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func scanSessionWithMembership(row pgx.Row) (model.NetworkSession, error) {
	var out model.NetworkSession
	err := row.Scan(&out.ID, &out.ProjectID, &out.Ref.SourceCode, &out.Ref.SourceInstance,
		&out.Ref.RecordType, &out.Ref.ExternalID, &out.Ref.TimeRange.From, &out.Ref.TimeRange.To,
		&out.Title, &out.Severity, &out.StartedAt, &out.EndedAt, &out.SourceRef, &out.FetchedAt, &out.Normalized,
		&out.Provenance, &out.ContextStatus, &out.ContextErrors, &out.CreatedAt, &out.UpdatedAt,
		&out.Direct, &out.Derived, &out.AttachedAt)
	return out, err
}

func (d *DB) InvestigationSessions(ctx context.Context, projectID, investigationID string, filter model.ObjectFilter) ([]model.NetworkSession, error) {
	if exists, err := d.InvestigationExists(ctx, projectID, investigationID); err != nil {
		return nil, err
	} else if !exists {
		return nil, store.ErrInvestigationNotFound
	}
	var cursorTime, cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	rows, err := d.Pgx().Query(ctx, `SELECT `+sessionColumns+`,m.directly_added,m.derived,m.attached_at
		FROM investigation_sessions m JOIN network_sessions s ON s.id=m.session_id AND s.project_id=m.project_id
		WHERE m.investigation_id=$1::uuid AND m.project_id=$2
		  AND ($3::text IS NULL OR s.severity=$3)
		  AND ($4::text IS NULL OR s.context_status=$4)
		  AND ($5::timestamptz IS NULL OR (m.attached_at,s.id)<($5,$6::uuid))
		ORDER BY m.attached_at DESC,s.id DESC LIMIT $7`, investigationID, projectID,
		filter.Severity, filter.ContextStatus, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.NetworkSession
	for rows.Next() {
		item, err := scanSessionWithMembership(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) GetSession(ctx context.Context, projectID, sessionID string) (model.NetworkSession, error) {
	out, err := scanSession(d.Pgx().QueryRow(ctx, `SELECT `+sessionColumns+` FROM network_sessions s WHERE s.id=$1::uuid AND s.project_id=$2`, sessionID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.NetworkSession{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.NetworkSession{}, fmt.Errorf("get session: %w", mapConstraint(err))
	}
	rows, err := d.Pgx().Query(ctx, `SELECT m.investigation_id::text
		FROM investigation_sessions m JOIN investigations i
		  ON i.id=m.investigation_id AND i.project_id=m.project_id AND i.is_deleted=false
		WHERE m.session_id=$1::uuid AND m.project_id=$2 ORDER BY m.investigation_id`, sessionID, projectID)
	if err != nil {
		return model.NetworkSession{}, fmt.Errorf("list session investigations: %w", mapConstraint(err))
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.NetworkSession{}, err
		}
		out.InvestigationIDs = append(out.InvestigationIDs, id)
	}
	return out, rows.Err()
}

func (d *DB) DetachSession(ctx context.Context, projectID, investigationID, sessionID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if exists, err := investigationExistsTx(ctx, tx, projectID, investigationID); err != nil {
		return err
	} else if !exists {
		return store.ErrInvestigationNotFound
	}
	var direct, derived bool
	err = tx.QueryRow(ctx, `SELECT directly_added,derived FROM investigation_sessions
		WHERE investigation_id=$1::uuid AND session_id=$2::uuid AND project_id=$3 FOR UPDATE`, investigationID, sessionID, projectID).Scan(&direct, &derived)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("lock session membership: %w", mapConstraint(err))
	}
	if derived {
		_, err = tx.Exec(ctx, `UPDATE investigation_sessions SET directly_added=false
			WHERE investigation_id=$1::uuid AND session_id=$2::uuid AND project_id=$3`, investigationID, sessionID, projectID)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM investigation_sessions
			WHERE investigation_id=$1::uuid AND session_id=$2::uuid AND project_id=$3`, investigationID, sessionID, projectID)
	}
	if err != nil {
		return fmt.Errorf("detach session: %w", mapConstraint(err))
	}
	if err := cleanupDerivedMembershipsTx(ctx, tx, projectID, investigationID); err != nil {
		return err
	}
	if err := cleanupExclusiveGranularContextTx(ctx, tx, projectID, investigationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
