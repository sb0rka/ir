package psql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const findingColumns = `
	f.id::text,f.project_id,f.source_code,f.source_instance,f.record_type,f.external_id,
	f.time_from,f.time_to,f.kind,f.title,f.description,f.severity,f.occurred_at,f.status,
	f.source_ref,f.fetched_at,f.normalized_snapshot,f.provenance,f.context_status,f.context_errors,
	f.created_at,f.updated_at`

func scanFinding(row pgx.Row) (model.Finding, error) {
	var out model.Finding
	err := row.Scan(&out.ID, &out.ProjectID, &out.Ref.SourceCode, &out.Ref.SourceInstance,
		&out.Ref.RecordType, &out.Ref.ExternalID, &out.Ref.TimeRange.From, &out.Ref.TimeRange.To,
		&out.Kind, &out.Title, &out.Description, &out.Severity, &out.OccurredAt, &out.Status,
		&out.SourceRef, &out.FetchedAt, &out.Normalized, &out.Provenance, &out.ContextStatus, &out.ContextErrors,
		&out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func (d *DB) InvestigationFindings(ctx context.Context, projectID, investigationID string, filter model.ObjectFilter) ([]model.Finding, error) {
	if exists, err := d.InvestigationExists(ctx, projectID, investigationID); err != nil {
		return nil, err
	} else if !exists {
		return nil, store.ErrInvestigationNotFound
	}
	var cursorTime, cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	rows, err := d.Pgx().Query(ctx, `SELECT `+findingColumns+`,m.directly_added,m.derived,m.attached_at
		FROM investigation_findings m JOIN findings f ON f.id=m.finding_id AND f.project_id=m.project_id
		WHERE m.investigation_id=$1::uuid AND m.project_id=$2
		  AND ($3::text IS NULL OR f.record_type=$3)
		  AND ($4::text IS NULL OR f.severity=$4)
		  AND ($5::text IS NULL OR f.context_status=$5)
		  AND ($6::timestamptz IS NULL OR (m.attached_at,f.id)<($6,$7::uuid))
		ORDER BY m.attached_at DESC,f.id DESC LIMIT $8`, investigationID, projectID,
		filter.RecordType, filter.Severity, filter.ContextStatus, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list findings: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.Finding
	for rows.Next() {
		item, err := scanFindingWithMembership(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanFindingWithMembership(row pgx.Row) (model.Finding, error) {
	var out model.Finding
	err := row.Scan(&out.ID, &out.ProjectID, &out.Ref.SourceCode, &out.Ref.SourceInstance,
		&out.Ref.RecordType, &out.Ref.ExternalID, &out.Ref.TimeRange.From, &out.Ref.TimeRange.To,
		&out.Kind, &out.Title, &out.Description, &out.Severity, &out.OccurredAt, &out.Status,
		&out.SourceRef, &out.FetchedAt, &out.Normalized, &out.Provenance, &out.ContextStatus, &out.ContextErrors,
		&out.CreatedAt, &out.UpdatedAt, &out.Direct, &out.Derived, &out.AttachedAt)
	return out, err
}

func (d *DB) GetFinding(ctx context.Context, projectID, findingID string) (model.Finding, error) {
	out, err := scanFinding(d.Pgx().QueryRow(ctx, `SELECT `+findingColumns+` FROM findings f WHERE f.id=$1::uuid AND f.project_id=$2`, findingID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Finding{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.Finding{}, fmt.Errorf("get finding: %w", mapConstraint(err))
	}
	rows, err := d.Pgx().Query(ctx, `SELECT investigation_id::text FROM investigation_findings WHERE finding_id=$1::uuid AND project_id=$2 ORDER BY investigation_id`, findingID, projectID)
	if err != nil {
		return model.Finding{}, fmt.Errorf("list finding investigations: %w", mapConstraint(err))
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return model.Finding{}, err
		}
		out.InvestigationIDs = append(out.InvestigationIDs, id)
	}
	return out, rows.Err()
}

func (d *DB) DetachFinding(ctx context.Context, projectID, investigationID, findingID string) error {
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
	err = tx.QueryRow(ctx, `SELECT directly_added,derived FROM investigation_findings
		WHERE investigation_id=$1::uuid AND finding_id=$2::uuid AND project_id=$3 FOR UPDATE`, investigationID, findingID, projectID).Scan(&direct, &derived)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("lock finding membership: %w", mapConstraint(err))
	}
	if derived {
		_, err = tx.Exec(ctx, `UPDATE investigation_findings SET directly_added=false
			WHERE investigation_id=$1::uuid AND finding_id=$2::uuid AND project_id=$3`, investigationID, findingID, projectID)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM investigation_findings
			WHERE investigation_id=$1::uuid AND finding_id=$2::uuid AND project_id=$3`, investigationID, findingID, projectID)
	}
	if err != nil {
		return fmt.Errorf("detach finding: %w", mapConstraint(err))
	}
	if err := cleanupDerivedMembershipsTx(ctx, tx, projectID, investigationID); err != nil {
		return err
	}
	if err := cleanupExclusiveGranularContextTx(ctx, tx, projectID, investigationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
