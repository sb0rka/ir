package psql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

const hypothesisSelect = `SELECT h.id::text,h.project_id,h.investigation_id::text,h.statement,h.description,h.status,h.reason,h.origin,h.version,h.created_at,h.updated_at,h.resolved_at FROM hypotheses h JOIN investigations i ON i.id=h.investigation_id AND i.project_id=h.project_id AND i.is_deleted=false AND h.is_deleted=false`

func scanHypothesis(row pgx.Row) (model.Hypothesis, error) {
	var h model.Hypothesis
	err := row.Scan(&h.ID, &h.ProjectID, &h.InvestigationID, &h.Statement, &h.Description,
		&h.Status, &h.Reason, &h.Origin, &h.Version, &h.CreatedAt, &h.UpdatedAt, &h.ResolvedAt)
	return h, err
}

func hypothesisByIDTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, hypothesisID string, lock bool) (model.Hypothesis, error) {
	query := hypothesisSelect + ` WHERE h.id=$1::uuid AND h.investigation_id=$2::uuid AND h.project_id=$3`
	if lock {
		query += ` FOR UPDATE OF h`
	}
	h, err := scanHypothesis(tx.QueryRow(ctx, query, hypothesisID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Hypothesis{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("get hypothesis: %w", mapConstraint(err))
	}
	return h, nil
}

func writableHypothesisTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, hypothesisID string, requireActive bool) (model.Hypothesis, error) {
	h, err := hypothesisByIDTx(ctx, tx, projectID, investigationID, hypothesisID, true)
	if err != nil {
		return model.Hypothesis{}, err
	}
	if h.Status == "resolved" || requireActive && h.Status != "active" {
		return model.Hypothesis{}, &store.ConflictError{IDs: []string{hypothesisID}}
	}
	return h, nil
}

func (d *DB) CreateHypothesis(ctx context.Context, input model.HypothesisNew) (model.Hypothesis, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("begin hypothesis: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, input.ProjectID, input.InvestigationID)
	if err != nil {
		return model.Hypothesis{}, err
	}
	if !exists {
		return model.Hypothesis{}, store.ErrInvestigationNotFound
	}
	statement := strings.TrimSpace(input.Statement)
	if statement == "" || utf8.RuneCountInString(statement) > 255 {
		return model.Hypothesis{}, store.ErrInvalidValue
	}
	description := trimOptional(input.Description)
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO hypotheses
		(project_id,investigation_id,statement,description,status,origin,version)
		VALUES ($1,$2::uuid,$3,$4,'proposed','analyst',1) RETURNING id::text`,
		input.ProjectID, input.InvestigationID, statement, description).Scan(&id)
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("create hypothesis: %w", mapConstraint(err))
	}
	out, err := hypothesisByIDTx(ctx, tx, input.ProjectID, input.InvestigationID, id, false)
	if err != nil {
		return model.Hypothesis{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Hypothesis{}, fmt.Errorf("commit hypothesis: %w", err)
	}
	return out, nil
}

func (d *DB) ListHypotheses(ctx context.Context, projectID, investigationID string, filter model.HypothesisFilter) ([]model.Hypothesis, error) {
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
	rows, err := d.Pgx().Query(ctx, hypothesisSelect+`
		WHERE h.project_id=$1 AND h.investigation_id=$2::uuid
		  AND ($3::text IS NULL OR h.status=$3)
		  AND ($4::timestamptz IS NULL OR (h.created_at,h.id)<($4,$5::uuid))
		ORDER BY h.created_at DESC,h.id DESC LIMIT $6`,
		projectID, investigationID, filter.Status, cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", mapConstraint(err))
	}
	defer rows.Close()
	out := make([]model.Hypothesis, 0)
	for rows.Next() {
		h, err := scanHypothesis(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (d *DB) GetHypothesis(ctx context.Context, projectID, investigationID, hypothesisID string) (model.Hypothesis, error) {
	h, err := scanHypothesis(d.Pgx().QueryRow(ctx, hypothesisSelect+`
		WHERE h.id=$1::uuid AND h.investigation_id=$2::uuid AND h.project_id=$3`,
		hypothesisID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Hypothesis{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("get hypothesis: %w", mapConstraint(err))
	}
	return h, nil
}

func (d *DB) UpdateHypothesis(ctx context.Context, patch model.HypothesisPatch) (model.Hypothesis, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("begin hypothesis update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	h, err := hypothesisByIDTx(ctx, tx, patch.ProjectID, patch.InvestigationID, patch.HypothesisID, true)
	if err != nil {
		return model.Hypothesis{}, err
	}
	if h.Version != patch.Version {
		return model.Hypothesis{}, &store.ConflictError{IDs: []string{patch.HypothesisID}}
	}
	changed := patch.Statement != nil || patch.HasDescription || patch.Status != nil || patch.Reason != nil
	if !changed {
		return h, nil
	}

	statement := h.Statement
	if patch.Statement != nil {
		statement = strings.TrimSpace(*patch.Statement)
		if statement == "" || utf8.RuneCountInString(statement) > 255 {
			return model.Hypothesis{}, store.ErrInvalidValue
		}
	}
	description := h.Description
	if patch.HasDescription {
		description = trimOptional(patch.Description)
	}
	status := h.Status
	if patch.Status != nil {
		status = strings.TrimSpace(*patch.Status)
		if !validHypothesisTransition(h.Status, status) {
			return model.Hypothesis{}, store.ErrInvalidValue
		}
	}
	reason := h.Reason
	resolvedAt := h.ResolvedAt
	if status == "resolved" {
		if patch.Reason != nil {
			value := strings.TrimSpace(*patch.Reason)
			if value == "" {
				return model.Hypothesis{}, store.ErrInvalidValue
			}
			reason = &value
		}
		if reason == nil || strings.TrimSpace(*reason) == "" {
			return model.Hypothesis{}, store.ErrInvalidValue
		}
		if h.Status != "resolved" {
			now := time.Now().UTC()
			resolvedAt = &now
		}
	} else {
		if patch.Reason != nil && strings.TrimSpace(*patch.Reason) != "" {
			return model.Hypothesis{}, store.ErrInvalidValue
		}
		reason = nil
		resolvedAt = nil
	}

	tag, err := tx.Exec(ctx, `UPDATE hypotheses SET
		statement=$1,description=$2,status=$3,reason=$4,resolved_at=$5,version=version+1
		WHERE id=$6::uuid AND investigation_id=$7::uuid AND project_id=$8 AND is_deleted=false`,
		statement, description, status, reason, resolvedAt,
		patch.HypothesisID, patch.InvestigationID, patch.ProjectID)
	if err != nil {
		return model.Hypothesis{}, fmt.Errorf("update hypothesis: %w", mapConstraint(err))
	}
	if tag.RowsAffected() == 0 {
		return model.Hypothesis{}, store.ErrRecordNotFound
	}
	out, err := hypothesisByIDTx(ctx, tx, patch.ProjectID, patch.InvestigationID, patch.HypothesisID, false)
	if err != nil {
		return model.Hypothesis{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Hypothesis{}, fmt.Errorf("commit hypothesis update: %w", err)
	}
	return out, nil
}

func validHypothesisTransition(from, to string) bool {
	if from == to {
		return from == "proposed" || from == "active" || from == "resolved"
	}
	switch from {
	case "proposed":
		return to == "active" || to == "resolved"
	case "active":
		return to == "resolved"
	case "resolved":
		return to == "active"
	default:
		return false
	}
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (d *DB) DeleteHypothesis(ctx context.Context, projectID, investigationID, hypothesisID string) error {
	tag, err := d.Pgx().Exec(ctx, `UPDATE hypotheses h SET is_deleted=true
		FROM investigations i
		WHERE h.id=$1::uuid AND h.investigation_id=$2::uuid AND h.project_id=$3
		  AND h.is_deleted=false AND i.id=h.investigation_id
		  AND i.project_id=h.project_id AND i.is_deleted=false`,
		hypothesisID, investigationID, projectID)
	if err != nil {
		return fmt.Errorf("soft-delete hypothesis: %w", mapConstraint(err))
	}
	if tag.RowsAffected() == 0 {
		return store.ErrRecordNotFound
	}
	return nil
}

func insertHypothesisNodeMembershipTx(ctx context.Context, tx pgx.Tx, hypothesisID, investigationID, nodeID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO hypothesis_nodes (hypothesis_id,investigation_id,node_id)
		VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, hypothesisID, investigationID, nodeID)
	if err != nil {
		return fmt.Errorf("add hypothesis node: %w", mapConstraint(err))
	}
	return nil
}

func insertHypothesisEdgeMembershipTx(ctx context.Context, tx pgx.Tx, hypothesisID, investigationID string, edge model.GraphEdge) error {
	if err := insertHypothesisNodeMembershipTx(ctx, tx, hypothesisID, investigationID, edge.SourceNodeID); err != nil {
		return err
	}
	if err := insertHypothesisNodeMembershipTx(ctx, tx, hypothesisID, investigationID, edge.TargetNodeID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO hypothesis_edges (hypothesis_id,investigation_id,edge_id)
		VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, hypothesisID, investigationID, edge.ID)
	if err != nil {
		return fmt.Errorf("add hypothesis edge: %w", mapConstraint(err))
	}
	return nil
}

func graphNodePathTx(ctx context.Context, tx pgx.Tx, projectID, investigationID, nodeID string) (model.GraphNode, error) {
	n, err := scanGraphNode(tx.QueryRow(ctx, graphNodeSelect+`
		WHERE n.id=$1::uuid AND n.investigation_id=$2::uuid AND i.project_id=$3`, nodeID, investigationID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.GraphNode{}, store.ErrRecordNotFound
	}
	if err != nil {
		return model.GraphNode{}, fmt.Errorf("get hypothesis node: %w", mapConstraint(err))
	}
	return n, nil
}

func (d *DB) AddHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return err
	}
	node, err := graphNodePathTx(ctx, tx, projectID, investigationID, nodeID)
	if err != nil {
		return err
	}
	if err := insertHypothesisNodeMembershipTx(ctx, tx, hypothesisID, investigationID, node.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) DeleteHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return err
	}
	if _, err := graphNodePathTx(ctx, tx, projectID, investigationID, nodeID); err != nil {
		return err
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hypothesis_nodes
		WHERE hypothesis_id=$1::uuid AND investigation_id=$2::uuid AND node_id=$3::uuid)`,
		hypothesisID, investigationID, nodeID).Scan(&member); err != nil {
		return err
	}
	if !member {
		return store.ErrRecordNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM hypothesis_edges he USING edges e
		WHERE he.hypothesis_id=$1::uuid AND he.investigation_id=$2::uuid
		  AND he.edge_id=e.id AND e.investigation_id=$2::uuid
		  AND (e.source_node_id=$3::uuid OR e.target_node_id=$3::uuid)`,
		hypothesisID, investigationID, nodeID); err != nil {
		return fmt.Errorf("delete incident hypothesis edges: %w", mapConstraint(err))
	}
	if _, err := tx.Exec(ctx, `DELETE FROM hypothesis_nodes
		WHERE hypothesis_id=$1::uuid AND investigation_id=$2::uuid AND node_id=$3::uuid`,
		hypothesisID, investigationID, nodeID); err != nil {
		return fmt.Errorf("delete hypothesis node: %w", mapConstraint(err))
	}
	return tx.Commit(ctx)
}

func (d *DB) AddHypothesisEdge(ctx context.Context, projectID, investigationID, hypothesisID, edgeID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return err
	}
	edge, err := graphEdgeByIDTx(ctx, tx, projectID, investigationID, edgeID, false)
	if err != nil {
		return err
	}
	if err := insertHypothesisEdgeMembershipTx(ctx, tx, hypothesisID, investigationID, edge); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) DeleteHypothesisEdge(ctx context.Context, projectID, investigationID, hypothesisID, edgeID string) error {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return err
	}
	if _, err := graphEdgeByIDTx(ctx, tx, projectID, investigationID, edgeID, false); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM hypothesis_edges
		WHERE hypothesis_id=$1::uuid AND investigation_id=$2::uuid AND edge_id=$3::uuid`,
		hypothesisID, investigationID, edgeID)
	if err != nil {
		return fmt.Errorf("delete hypothesis edge: %w", mapConstraint(err))
	}
	if tag.RowsAffected() == 0 {
		return store.ErrRecordNotFound
	}
	return tx.Commit(ctx)
}

func (d *DB) HypothesisGraph(ctx context.Context, projectID, investigationID, hypothesisID string, filter model.EdgeFilter) (model.HypothesisGraph, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.HypothesisGraph{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := hypothesisByIDTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return model.HypothesisGraph{}, err
	}
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{"proposed", "confirmed"}
	}
	edgeRows, err := tx.Query(ctx, graphEdgeSelect+`
		JOIN hypothesis_edges he ON he.edge_id=e.id AND he.investigation_id=e.investigation_id
		WHERE he.hypothesis_id=$1::uuid AND e.investigation_id=$2::uuid AND i.project_id=$3
		  AND e.status=ANY($4::text[])
		  AND ($5::real IS NULL OR e.confidence >= $5)
		ORDER BY e.created_at,e.id`, hypothesisID, investigationID, projectID, statuses, filter.MinConfidence)
	if err != nil {
		return model.HypothesisGraph{}, fmt.Errorf("list hypothesis edges: %w", mapConstraint(err))
	}
	out := model.HypothesisGraph{Edges: make([]model.GraphEdge, 0), Nodes: make([]model.GraphNode, 0)}
	for edgeRows.Next() {
		edge, err := scanGraphEdge(edgeRows)
		if err != nil {
			edgeRows.Close()
			return model.HypothesisGraph{}, err
		}
		out.Edges = append(out.Edges, edge)
	}
	if err := edgeRows.Err(); err != nil {
		edgeRows.Close()
		return model.HypothesisGraph{}, err
	}
	edgeRows.Close()

	nodeRows, err := tx.Query(ctx, graphNodeSelect+`
		WHERE n.investigation_id=$2::uuid AND i.project_id=$3 AND (
		  EXISTS (SELECT 1 FROM hypothesis_nodes hn
		          WHERE hn.hypothesis_id=$1::uuid AND hn.investigation_id=n.investigation_id AND hn.node_id=n.id)
		  OR EXISTS (SELECT 1 FROM hypothesis_edges he JOIN edges member_edge
		               ON member_edge.id=he.edge_id AND member_edge.investigation_id=he.investigation_id
		             WHERE he.hypothesis_id=$1::uuid AND he.investigation_id=n.investigation_id
		               AND member_edge.status=ANY($4::text[])
		               AND ($5::real IS NULL OR member_edge.confidence >= $5)
		               AND (member_edge.source_node_id=n.id OR member_edge.target_node_id=n.id))
		)
		ORDER BY n.created_at,n.id`, hypothesisID, investigationID, projectID, statuses, filter.MinConfidence)
	if err != nil {
		return model.HypothesisGraph{}, fmt.Errorf("list hypothesis nodes: %w", mapConstraint(err))
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		node, err := scanGraphNode(nodeRows)
		if err != nil {
			return model.HypothesisGraph{}, err
		}
		out.Nodes = append(out.Nodes, node)
	}
	if err := nodeRows.Err(); err != nil {
		return model.HypothesisGraph{}, err
	}
	return out, nil
}

func (d *DB) CreateHypothesisNode(ctx context.Context, projectID, investigationID, hypothesisID, nodeType string, entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphNode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, projectID, investigationID, hypothesisID, false); err != nil {
		return model.GraphNode{}, err
	}
	node, _, err := upsertNodeTx(ctx, tx, investigationID, nodeType, entityID, eventID, origin, somIssueIDs)
	if err != nil {
		return model.GraphNode{}, err
	}
	if err := insertHypothesisNodeMembershipTx(ctx, tx, hypothesisID, investigationID, node.ID); err != nil {
		return model.GraphNode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphNode{}, err
	}
	return node, nil
}

func (d *DB) CreateHypothesisGraphEdge(ctx context.Context, hypothesisID string, input model.GraphEdgeNew) (model.GraphEdge, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.GraphEdge{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := writableHypothesisTx(ctx, tx, input.ProjectID, input.InvestigationID, hypothesisID, false); err != nil {
		return model.GraphEdge{}, err
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
	if err := insertHypothesisEdgeMembershipTx(ctx, tx, hypothesisID, input.InvestigationID, edge); err != nil {
		return model.GraphEdge{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.GraphEdge{}, fmt.Errorf("commit hypothesis graph edge: %w", err)
	}
	return edge, nil
}
