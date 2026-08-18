package psql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const graphNodeSelect = `SELECT n.id::text,n.investigation_id::text,n.node_type,n.entity_id::text,n.event_id::text,n.origin,COALESCE((SELECT array_agg(som_issue_id::text ORDER BY som_issue_id) FROM graph_node_som_issues WHERE graph_node_id=n.id),'{}'),COALESCE(ev.title,en.display_name,en.canonical_key),en.type_code,en.canonical_key,ev.occurred_at,n.created_at FROM graph_nodes n JOIN investigations i ON i.id=n.investigation_id LEFT JOIN events ev ON ev.id=n.event_id LEFT JOIN entities en ON en.id=n.entity_id`

func scanGraphNode(row pgx.Row) (model.GraphNode, error) {
	var n model.GraphNode
	err := row.Scan(&n.ID, &n.InvestigationID, &n.NodeType, &n.EntityID, &n.EventID, &n.Origin, &n.SomIssueIDs, &n.Label, &n.TypeCode, &n.CanonicalKey, &n.OccurredAt, &n.CreatedAt)
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
	n, _, err := upsertNodeTx(ctx, tx, investigationID, nodeType, entityID, eventID, origin, somIssueIDs)
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
		prefix = `WITH RECURSIVE inv_scope(id) AS (SELECT id FROM investigations WHERE id=$1::uuid AND project_id=$2 UNION ALL SELECT child.id FROM investigations child JOIN inv_scope parent ON child.parent_id=parent.id WHERE child.project_id=$2) `
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

const graphEdgeSelect = `SELECT e.id::text,e.investigation_id::text,e.source_node_id::text,e.target_node_id::text,e.relation_code,e.status,e.reject_reason,e.confidence,e.why,e.origin,e.origin_ref,COALESCE((SELECT array_agg(event_id::text ORDER BY event_id) FROM edge_evidence WHERE edge_id=e.id),'{}'),e.metadata,e.version,e.created_at,e.updated_at FROM edges e JOIN investigations i ON i.id=e.investigation_id`

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
		prefix = `WITH RECURSIVE inv_scope(id) AS (SELECT id FROM investigations WHERE id=$1::uuid AND project_id=$2 UNION ALL SELECT child.id FROM investigations child JOIN inv_scope parent ON child.parent_id=parent.id WHERE child.project_id=$2) `
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
