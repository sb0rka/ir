package psql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const graphNodeSelect = `SELECT n.id::text,n.investigation_id::text,n.node_type,n.entity_id::text,n.event_id::text,n.origin,n.why,COALESCE((SELECT array_agg(som_issue_id::text ORDER BY som_issue_id) FROM graph_node_som_issues WHERE graph_node_id=n.id),'{}'),COALESCE(ev.title,en.display_name,en.canonical_key),en.type_code,en.canonical_key,ev.occurred_at,n.created_at FROM graph_nodes n JOIN investigations i ON i.id=n.investigation_id AND i.is_deleted=false LEFT JOIN events ev ON ev.id=n.event_id LEFT JOIN entities en ON en.id=n.entity_id`

func scanGraphNode(row pgx.Row) (model.GraphNode, error) {
	var n model.GraphNode
	err := row.Scan(&n.ID, &n.InvestigationID, &n.NodeType, &n.EntityID, &n.EventID, &n.Origin, &n.Why, &n.SomIssueIDs, &n.Label, &n.TypeCode, &n.CanonicalKey, &n.OccurredAt, &n.CreatedAt)
	return n, err
}

func graphNodeByIDTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, nodeID string) (model.GraphNode, error) {
	n, err := scanGraphNode(tx.QueryRow(ctx, graphNodeSelect+` WHERE n.id=$1::uuid AND n.investigation_id=$2::uuid AND ($3='' OR i.project_id=$3)`, nodeID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphNode{}, store.ErrUnknownReference
	}
	if err != nil {
		return model.GraphNode{}, fmt.Errorf("get graph node: %w", mapConstraint(err))
	}
	return n, nil
}

func (d *DB) CreateNode(ctx context.Context, projectID, investigationID, nodeType string, entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphNode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, projectID, investigationID)
	if err != nil {
		return model.GraphNode{}, err
	}
	if !exists {
		return model.GraphNode{}, store.ErrInvestigationNotFound
	}
	n, _, err := upsertNodeTx(ctx, tx, investigationID, nodeType, entityID, eventID, origin, nil, somIssueIDs)
	if err != nil {
		return model.GraphNode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphNode{}, err
	}
	return n, nil
}

func (d *DB) GraphNodes(ctx context.Context, projectID, investigationID string, filter model.NodeFilter) ([]model.GraphNode, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, store.ErrInvestigationNotFound
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10000
	}
	var cursorTime, cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	prefix := ""
	scopeClause := "n.investigation_id=$1::uuid"
	if filter.IncludeSubtree {
		prefix = `WITH RECURSIVE inv_scope(id) AS (SELECT id FROM investigations WHERE id=$1::uuid AND project_id=$2 AND is_deleted=false UNION ALL SELECT child.id FROM investigations child JOIN inv_scope parent ON child.parent_id=parent.id WHERE child.project_id=$2 AND child.is_deleted=false) `
		scopeClause = "n.investigation_id IN (SELECT id FROM inv_scope)"
	}
	rows, err := d.Pgx().Query(ctx, prefix+graphNodeSelect+` WHERE `+scopeClause+` AND i.project_id=$2 AND ($3::text IS NULL OR n.node_type=$3) AND ($4::text IS NULL OR ev.title ILIKE '%'||$4||'%' OR en.display_name ILIKE '%'||$4||'%' OR en.canonical_key ILIKE '%'||$4||'%') AND ($5::timestamptz IS NULL OR (n.created_at,n.id)>($5,$6::uuid)) ORDER BY n.created_at,n.id LIMIT $7`, investigationID, projectID, filter.NodeType, filter.Q, cursorTime, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list graph nodes: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.GraphNode
	for rows.Next() {
		n, err := scanGraphNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (d *DB) GetNode(ctx context.Context, projectID, investigationID, nodeID string) (model.GraphNode, error) {
	n, err := scanGraphNode(d.Pgx().QueryRow(ctx, graphNodeSelect+` WHERE n.id=$1::uuid AND n.investigation_id=$2::uuid AND i.project_id=$3`, nodeID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphNode{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.GraphNode{}, fmt.Errorf("get node: %w", mapConstraint(err))
	}
	return n, nil
}

const graphEdgeSelect = `SELECT e.id::text,e.investigation_id::text,e.source_node_id::text,e.target_node_id::text,e.relation_code,e.status,e.reject_reason,e.confidence,e.why,e.origin,e.origin_ref,COALESCE((SELECT array_agg(event_id::text ORDER BY event_id) FROM edge_evidence WHERE edge_id=e.id),'{}'),e.metadata,e.version,e.created_at,e.updated_at FROM edges e JOIN investigations i ON i.id=e.investigation_id AND i.is_deleted=false`

func scanGraphEdge(row pgx.Row) (model.GraphEdge, error) {
	var e model.GraphEdge
	err := row.Scan(&e.ID, &e.InvestigationID, &e.SourceNodeID, &e.TargetNodeID, &e.RelationCode, &e.Status, &e.RejectReason, &e.Confidence, &e.Why, &e.Origin, &e.OriginRef, &e.EvidenceEventIDs, &e.Metadata, &e.Version, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func (d *DB) GraphEdges(ctx context.Context, projectID, investigationID string, filter model.EdgeFilter) ([]model.GraphEdge, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, store.ErrInvestigationNotFound
	}
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{"proposed", "confirmed"}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10000
	}
	var cursorTime, cursorID any
	if filter.Cursor != nil {
		cursorTime, cursorID = filter.Cursor.Time, filter.Cursor.ID
	}
	prefix := ""
	scopeClause := "e.investigation_id=$1::uuid"
	if filter.IncludeSubtree {
		prefix = `WITH RECURSIVE inv_scope(id) AS (SELECT id FROM investigations WHERE id=$1::uuid AND project_id=$2 AND is_deleted=false UNION ALL SELECT child.id FROM investigations child JOIN inv_scope parent ON child.parent_id=parent.id WHERE child.project_id=$2 AND child.is_deleted=false) `
		scopeClause = "e.investigation_id IN (SELECT id FROM inv_scope)"
	}
	rows, err := d.Pgx().Query(ctx, prefix+graphEdgeSelect+` WHERE `+scopeClause+` AND i.project_id=$2 AND e.status=ANY($3::text[]) AND ($4::text IS NULL OR e.origin=$4) AND ($5::text IS NULL OR e.relation_code=$5) AND ($6::uuid IS NULL OR e.source_node_id=$6::uuid OR e.target_node_id=$6::uuid) AND ($7::real IS NULL OR e.confidence >= $7) AND ($8::timestamptz IS NULL OR (e.created_at,e.id)>($8,$9::uuid)) ORDER BY e.created_at,e.id LIMIT $10`, investigationID, projectID, statuses, filter.Origin, filter.RelationCode, filter.NodeID, filter.MinConfidence, cursorTime, cursorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list graph edges: %w", mapConstraint(err))
	}
	defer rows.Close()
	var out []model.GraphEdge
	for rows.Next() {
		e, err := scanGraphEdge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func graphEdgeByIDTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, edgeID string, lock bool) (model.GraphEdge, error) {
	query := graphEdgeSelect + ` WHERE e.id=$1::uuid AND e.investigation_id=$2::uuid AND i.project_id=$3`
	if lock {
		query += ` FOR UPDATE OF e`
	}
	edge, err := scanGraphEdge(tx.QueryRow(ctx, query, edgeID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphEdge{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.GraphEdge{}, fmt.Errorf("get graph edge: %w", mapConstraint(err))
	}
	return edge, nil
}

func normalizeEdgeNodesTx(ctx context.Context, tx pgx.Tx, source, target model.GraphNode, relationCode string) (model.GraphNode, model.GraphNode, bool, error) {
	var sourceKind, targetKind string
	var directed bool
	err := tx.QueryRow(ctx, `SELECT source_kind,target_kind,directed FROM relation_types WHERE code=$1`, strings.TrimSpace(relationCode)).Scan(&sourceKind, &targetKind, &directed)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphNode{}, model.GraphNode{}, false, store.ErrInvalidValue
	}
	if err != nil {
		return model.GraphNode{}, model.GraphNode{}, false, fmt.Errorf("get relation type: %w", mapConstraint(err))
	}
	if source.NodeType != sourceKind || target.NodeType != targetKind {
		return model.GraphNode{}, model.GraphNode{}, false, store.ErrInvalidValue
	}
	if !directed && source.ID > target.ID {
		source, target = target, source
	}
	return source, target, directed, nil
}

func uniqueIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validateEvidenceEventsTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string, eventIDs []string) ([]string, error) {
	ids := uniqueIDs(eventIDs)
	if len(ids) == 0 {
		return ids, nil
	}
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*)::int FROM investigation_events
		WHERE investigation_id=$1::uuid AND project_id=$2 AND event_id=ANY($3::uuid[])`, investigationID, projectID, ids).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("validate edge evidence: %w", mapConstraint(err))
	}
	if count != len(ids) {
		return nil, store.ErrUnknownReference
	}
	return ids, nil
}

func addEvidenceTx(ctx context.Context, tx pgx.Tx, investigationID, edgeID string, eventIDs []string) (int64, error) {
	var inserted int64
	for _, eventID := range eventIDs {
		tag, err := tx.Exec(ctx, `INSERT INTO edge_evidence (edge_id,event_id,investigation_id)
			VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, edgeID, eventID, investigationID)
		if err != nil {
			return 0, fmt.Errorf("add edge evidence: %w", mapConstraint(err))
		}
		inserted += tag.RowsAffected()
	}
	return inserted, nil
}

func (d *DB) CreateGraphEdge(ctx context.Context, input model.GraphEdgeNew) (model.GraphEdge, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphEdge{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, input.ProjectID, input.InvestigationID)
	if err != nil {
		return model.GraphEdge{}, err
	}
	if !exists {
		return model.GraphEdge{}, store.ErrInvestigationNotFound
	}
	source, err := graphNodeByIDTx(ctx, tx, input.ProjectID, input.InvestigationID, input.SourceNodeID)
	if err != nil {
		return model.GraphEdge{}, err
	}
	target, err := graphNodeByIDTx(ctx, tx, input.ProjectID, input.InvestigationID, input.TargetNodeID)
	if err != nil {
		return model.GraphEdge{}, err
	}
	evidenceIDs, err := validateEvidenceEventsTx(ctx, tx, input.ProjectID, input.InvestigationID, input.EvidenceEventIDs)
	if err != nil {
		return model.GraphEdge{}, err
	}
	edgeID, inserted, err := insertEdgeTx(ctx, tx, input.InvestigationID, source, target,
		strings.TrimSpace(input.RelationCode), "confirmed", "analyst", input.OriginRef,
		input.Confidence, input.Why, nil)
	if err != nil {
		return model.GraphEdge{}, err
	}
	if !inserted {
		return model.GraphEdge{}, &store.ConflictError{IDs: []string{edgeID}}
	}
	if _, err := addEvidenceTx(ctx, tx, input.InvestigationID, edgeID, evidenceIDs); err != nil {
		return model.GraphEdge{}, err
	}
	edge, err := graphEdgeByIDTx(ctx, tx, input.ProjectID, input.InvestigationID, edgeID, false)
	if err != nil {
		return model.GraphEdge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphEdge{}, fmt.Errorf("commit graph edge: %w", err)
	}
	return edge, nil
}

func (d *DB) GetGraphEdge(ctx context.Context, projectID, investigationID, edgeID string) (model.GraphEdge, error) {
	edge, err := scanGraphEdge(d.Pgx().QueryRow(ctx, graphEdgeSelect+` WHERE e.id=$1::uuid AND e.investigation_id=$2::uuid AND i.project_id=$3`, edgeID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphEdge{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.GraphEdge{}, fmt.Errorf("get graph edge: %w", mapConstraint(err))
	}
	return edge, nil
}

func evidenceCountTx(ctx context.Context, tx pgx.Tx, edgeID string) (int, error) {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM edge_evidence WHERE edge_id=$1::uuid`, edgeID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count edge evidence: %w", mapConstraint(err))
	}
	return count, nil
}

func (d *DB) UpdateGraphEdge(ctx context.Context, patch model.GraphEdgePatch) (model.GraphEdge, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphEdge{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	edge, err := graphEdgeByIDTx(ctx, tx, patch.ProjectID, patch.InvestigationID, patch.EdgeID, true)
	if err != nil {
		return model.GraphEdge{}, err
	}
	if edge.Version != patch.Version {
		return model.GraphEdge{}, &store.ConflictError{IDs: []string{patch.EdgeID}}
	}
	changed := patch.Status != nil || patch.RejectReason != nil || patch.Confidence != nil || patch.Why != nil || patch.HasMetadata
	if !changed {
		return edge, nil
	}
	status := edge.Status
	if patch.Status != nil {
		status = strings.TrimSpace(*patch.Status)
		if status != "proposed" && status != "confirmed" && status != "rejected" {
			return model.GraphEdge{}, store.ErrInvalidValue
		}
		if edge.Status != "proposed" && status == "proposed" {
			return model.GraphEdge{}, store.ErrInvalidValue
		}
	}
	rejectReason := edge.RejectReason
	if patch.RejectReason != nil {
		reason := strings.TrimSpace(*patch.RejectReason)
		if status != "rejected" || reason == "" {
			return model.GraphEdge{}, store.ErrInvalidValue
		}
		rejectReason = &reason
	}
	if patch.Status != nil && status == "rejected" && patch.RejectReason == nil {
		return model.GraphEdge{}, store.ErrInvalidValue
	}
	if status == "confirmed" {
		rejectReason = nil
		if edge.Status != "confirmed" && edge.Origin != "analyst" {
			count, err := evidenceCountTx(ctx, tx, edge.ID)
			if err != nil {
				return model.GraphEdge{}, err
			}
			if count == 0 {
				return model.GraphEdge{}, store.ErrInvalidValue
			}
		}
	}
	confidence := edge.Confidence
	if patch.Confidence != nil {
		if *patch.Confidence < 0 || *patch.Confidence > 1 {
			return model.GraphEdge{}, store.ErrInvalidValue
		}
		confidence = patch.Confidence
	}
	why := edge.Why
	if patch.Why != nil {
		value := strings.TrimSpace(*patch.Why)
		why = &value
	}
	metadata := edge.Metadata
	if patch.HasMetadata {
		if !json.Valid(patch.Metadata) {
			return model.GraphEdge{}, store.ErrInvalidValue
		}
		metadata = patch.Metadata
	}
	_, err = tx.Exec(ctx, `UPDATE edges SET status=$1,reject_reason=$2,confidence=$3,why=$4,metadata=$5::jsonb,version=version+1
		WHERE id=$6::uuid AND investigation_id=$7::uuid`, status, rejectReason, confidence, why, string(metadata), edge.ID, patch.InvestigationID)
	if err != nil {
		return model.GraphEdge{}, fmt.Errorf("update graph edge: %w", mapConstraint(err))
	}
	edge, err = graphEdgeByIDTx(ctx, tx, patch.ProjectID, patch.InvestigationID, patch.EdgeID, false)
	if err != nil {
		return model.GraphEdge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphEdge{}, fmt.Errorf("commit graph edge update: %w", err)
	}
	return edge, nil
}

func (d *DB) DeleteGraphEdge(ctx context.Context, projectID, investigationID, edgeID string) error {
	tag, err := d.Pgx().Exec(ctx, `DELETE FROM edges e USING investigations i
		WHERE e.id=$1::uuid AND e.investigation_id=$2::uuid AND i.id=e.investigation_id
		  AND i.project_id=$3 AND i.is_deleted=false`, edgeID, investigationID, projectID)
	if err != nil {
		return fmt.Errorf("delete graph edge: %w", mapConstraint(err))
	}
	if tag.RowsAffected() == 0 {
		return store.ErrRecordNotFound
	}
	return nil
}

func scanEvidenceEvents(rows pgx.Rows) ([]model.EvidenceEvent, error) {
	defer rows.Close()
	out := make([]model.EvidenceEvent, 0)
	for rows.Next() {
		var event model.EvidenceEvent
		if err := rows.Scan(&event.ID, &event.SourceCode, &event.SourceEventID, &event.SourceRef, &event.EventType, &event.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func evidenceEventsTx(ctx context.Context, tx pgx.Tx, investigationID, edgeID string) ([]model.EvidenceEvent, error) {
	rows, err := tx.Query(ctx, `SELECT ev.id::text,ev.source_code,ev.source_event_id,ev.source_ref,ev.event_type,ev.occurred_at
		FROM edge_evidence ee JOIN events ev ON ev.id=ee.event_id
		WHERE ee.edge_id=$1::uuid AND ee.investigation_id=$2::uuid
		ORDER BY ev.occurred_at,ev.id`, edgeID, investigationID)
	if err != nil {
		return nil, fmt.Errorf("list graph edge evidence: %w", mapConstraint(err))
	}
	return scanEvidenceEvents(rows)
}

func (d *DB) GraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID string) ([]model.EvidenceEvent, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := graphEdgeByIDTx(ctx, tx, projectID, investigationID, edgeID, false); err != nil {
		return nil, err
	}
	events, err := evidenceEventsTx(ctx, tx, investigationID, edgeID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (d *DB) AddGraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID string, eventIDs []string) ([]model.EvidenceEvent, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := graphEdgeByIDTx(ctx, tx, projectID, investigationID, edgeID, true); err != nil {
		return nil, err
	}
	ids, err := validateEvidenceEventsTx(ctx, tx, projectID, investigationID, eventIDs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, store.ErrInvalidValue
	}
	inserted, err := addEvidenceTx(ctx, tx, investigationID, edgeID, ids)
	if err != nil {
		return nil, err
	}
	if inserted > 0 {
		if _, err := tx.Exec(ctx, `UPDATE edges SET version=version+1 WHERE id=$1::uuid AND investigation_id=$2::uuid`, edgeID, investigationID); err != nil {
			return nil, fmt.Errorf("bump edge version: %w", mapConstraint(err))
		}
	}
	events, err := evidenceEventsTx(ctx, tx, investigationID, edgeID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (d *DB) DeleteGraphEdgeEvidence(ctx context.Context, projectID, investigationID, edgeID, eventID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	edge, err := graphEdgeByIDTx(ctx, tx, projectID, investigationID, edgeID, true)
	if err != nil {
		return err
	}
	var linked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM edge_evidence WHERE edge_id=$1::uuid AND event_id=$2::uuid AND investigation_id=$3::uuid)`, edgeID, eventID, investigationID).Scan(&linked); err != nil {
		return fmt.Errorf("get edge evidence: %w", mapConstraint(err))
	}
	if !linked {
		return store.ErrRecordNotFound
	}
	count, err := evidenceCountTx(ctx, tx, edgeID)
	if err != nil {
		return err
	}
	if count <= 1 && (edge.Status == "confirmed" || edge.Origin == "agent") {
		return store.ErrInvalidValue
	}
	if _, err := tx.Exec(ctx, `DELETE FROM edge_evidence WHERE edge_id=$1::uuid AND event_id=$2::uuid AND investigation_id=$3::uuid`, edgeID, eventID, investigationID); err != nil {
		return fmt.Errorf("delete edge evidence: %w", mapConstraint(err))
	}
	if _, err := tx.Exec(ctx, `UPDATE edges SET version=version+1 WHERE id=$1::uuid AND investigation_id=$2::uuid`, edgeID, investigationID); err != nil {
		return fmt.Errorf("bump edge version: %w", mapConstraint(err))
	}
	return tx.Commit(ctx)
}

type lockedReviewEdge struct {
	Status        string
	Origin        string
	Version       int
	EvidenceCount int
}

func validateReviewItems(request model.EdgeReviewRequest) ([]string, error) {
	if len(request.Confirm) == 0 && len(request.Reject) == 0 {
		return nil, store.ErrInvalidValue
	}
	ids := make([]string, 0, len(request.Confirm)+len(request.Reject))
	seen := make(map[string]struct{}, cap(ids))
	for _, item := range request.Confirm {
		if strings.TrimSpace(item.ID) == "" || item.Version < 1 {
			return nil, store.ErrInvalidValue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, store.ErrInvalidValue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	for _, item := range request.Reject {
		if strings.TrimSpace(item.ID) == "" || item.Version < 1 || item.Reason == nil || strings.TrimSpace(*item.Reason) == "" {
			return nil, store.ErrInvalidValue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, store.ErrInvalidValue
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
	}
	return ids, nil
}

func (d *DB) ReviewGraphEdges(ctx context.Context, request model.EdgeReviewRequest) (model.EdgeReviewResult, error) {
	ids, err := validateReviewItems(request)
	if err != nil {
		return model.EdgeReviewResult{}, err
	}
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.EdgeReviewResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, request.ProjectID, request.InvestigationID)
	if err != nil {
		return model.EdgeReviewResult{}, err
	}
	if !exists {
		return model.EdgeReviewResult{}, store.ErrInvestigationNotFound
	}
	lockIDs := append([]string(nil), ids...)
	sort.Strings(lockIDs)
	rows, err := tx.Query(ctx, `SELECT e.id::text,e.status,e.origin,e.version,
		(SELECT count(*)::int FROM edge_evidence ee WHERE ee.edge_id=e.id)
		FROM edges e JOIN investigations i ON i.id=e.investigation_id
		WHERE e.investigation_id=$1::uuid AND i.project_id=$2 AND i.is_deleted=false
		  AND e.id=ANY($3::uuid[])
		ORDER BY e.id FOR UPDATE OF e`, request.InvestigationID, request.ProjectID, lockIDs)
	if err != nil {
		return model.EdgeReviewResult{}, fmt.Errorf("lock graph edges for review: %w", mapConstraint(err))
	}
	locked := make(map[string]lockedReviewEdge, len(ids))
	for rows.Next() {
		var id string
		var edge lockedReviewEdge
		if err := rows.Scan(&id, &edge.Status, &edge.Origin, &edge.Version, &edge.EvidenceCount); err != nil {
			rows.Close()
			return model.EdgeReviewResult{}, err
		}
		locked[id] = edge
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.EdgeReviewResult{}, err
	}
	if len(locked) != len(ids) {
		return model.EdgeReviewResult{}, store.ErrRecordNotFound
	}
	conflicts := make([]string, 0)
	for _, item := range request.Confirm {
		if locked[item.ID].Version != item.Version {
			conflicts = append(conflicts, item.ID)
		}
	}
	for _, item := range request.Reject {
		if locked[item.ID].Version != item.Version {
			conflicts = append(conflicts, item.ID)
		}
	}
	if len(conflicts) > 0 {
		return model.EdgeReviewResult{}, &store.ConflictError{IDs: conflicts}
	}
	for _, item := range request.Confirm {
		edge := locked[item.ID]
		if edge.Status != "proposed" || (edge.Origin != "analyst" && edge.EvidenceCount == 0) {
			return model.EdgeReviewResult{}, store.ErrInvalidValue
		}
	}
	for _, item := range request.Reject {
		if locked[item.ID].Status != "proposed" {
			return model.EdgeReviewResult{}, store.ErrInvalidValue
		}
	}
	result := model.EdgeReviewResult{Confirmed: make([]string, 0, len(request.Confirm)), Rejected: make([]string, 0, len(request.Reject))}
	for _, item := range request.Confirm {
		if _, err := tx.Exec(ctx, `UPDATE edges SET status='confirmed',reject_reason=NULL,version=version+1 WHERE id=$1::uuid AND investigation_id=$2::uuid`, item.ID, request.InvestigationID); err != nil {
			return model.EdgeReviewResult{}, fmt.Errorf("confirm graph edge: %w", mapConstraint(err))
		}
		result.Confirmed = append(result.Confirmed, item.ID)
	}
	for _, item := range request.Reject {
		reason := strings.TrimSpace(*item.Reason)
		if _, err := tx.Exec(ctx, `UPDATE edges SET status='rejected',reject_reason=$1,version=version+1 WHERE id=$2::uuid AND investigation_id=$3::uuid`, reason, item.ID, request.InvestigationID); err != nil {
			return model.EdgeReviewResult{}, fmt.Errorf("reject graph edge: %w", mapConstraint(err))
		}
		result.Rejected = append(result.Rejected, item.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.EdgeReviewResult{}, fmt.Errorf("commit graph edge review: %w", err)
	}
	return result, nil
}

func (d *DB) AgentResultCounts(ctx context.Context, projectID, investigationID, somIssueID string) (int, int, error) {
	exists, err := d.InvestigationExists(ctx, projectID, investigationID)
	if err != nil {
		return 0, 0, err
	}
	if !exists {
		return 0, 0, store.ErrInvestigationNotFound
	}
	var nodes, edges int
	err = d.Pgx().QueryRow(ctx, `
		SELECT
		  (SELECT count(*)::int FROM graph_nodes n
		     JOIN investigations i ON i.id=n.investigation_id AND i.is_deleted=false
		     JOIN graph_node_som_issues g ON g.graph_node_id=n.id
		    WHERE n.investigation_id=$1::uuid AND i.project_id=$2 AND g.som_issue_id=$3::uuid),
		  (SELECT count(*)::int FROM edges e
		     JOIN investigations i ON i.id=e.investigation_id AND i.is_deleted=false
		    WHERE e.investigation_id=$1::uuid AND i.project_id=$2 AND e.origin='agent' AND e.origin_ref=$3)`,
		investigationID, projectID, somIssueID).Scan(&nodes, &edges)
	if err != nil {
		return 0, 0, fmt.Errorf("agent result counts: %w", mapConstraint(err))
	}
	return nodes, edges, nil
}
