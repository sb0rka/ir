package psql

import (
	"context"
	"math"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/grouping"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

// GraphProjection reads raw evidence and group decisions in one consistent snapshot.
func (d *DB) GraphProjection(ctx context.Context, r model.ProjectionRequest) (model.GraphProjection, error) {
	var empty model.GraphProjection
	if r.Filter.MinConfidence != nil && (math.IsNaN(float64(*r.Filter.MinConfidence)) || *r.Filter.MinConfidence < 0 || *r.Filter.MinConfidence > 1) {
		return empty, store.ErrInvalidValue
	}
	statuses := r.Filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{"proposed", "confirmed"}
	}
	for _, s := range statuses {
		if !slices.Contains([]string{"proposed", "confirmed", "rejected"}, s) {
			return empty, store.ErrInvalidValue
		}
	}
	if r.HypothesisID != nil && r.Filter.IncludeSubtree {
		return empty, store.ErrInvalidValue
	}
	tx, err := d.Pgx().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	scope, err := groupScopeTx(ctx, tx, r.ProjectID, r.InvestigationID, false)
	if err != nil {
		return empty, err
	}
	if r.HypothesisID != nil {
		if _, err = hypothesisByIDTx(ctx, tx, r.ProjectID, r.InvestigationID, *r.HypothesisID, false); err != nil {
			return empty, err
		}
	}
	prefix := `WITH RECURSIVE view_scope AS (
 SELECT id FROM investigations WHERE project_id=$1 AND id=$2::uuid AND is_deleted=false
 UNION ALL SELECT i.id FROM investigations i JOIN view_scope p ON i.parent_id=p.id WHERE i.project_id=$1 AND i.is_deleted=false AND $3::boolean
) `
	edgeSQL := prefix + graphEdgeSelect + ` WHERE i.project_id=$1 AND e.investigation_id IN (SELECT id FROM view_scope)
 AND e.status=ANY($4::text[]) AND ($5::real IS NULL OR e.confidence >= $5)
 AND ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM hypothesis_edges he WHERE he.hypothesis_id=$6 AND he.investigation_id=e.investigation_id AND he.edge_id=e.id)) ORDER BY e.id`
	rows, err := tx.Query(ctx, edgeSQL, r.ProjectID, r.InvestigationID, r.Filter.IncludeSubtree, statuses, r.Filter.MinConfidence, r.HypothesisID)
	if err != nil {
		return empty, err
	}
	edges := []model.GraphEdge{}
	endpointIDs := []string{}
	for rows.Next() {
		e, eErr := scanGraphEdge(rows)
		if eErr != nil {
			rows.Close()
			return empty, eErr
		}
		edges = append(edges, e)
		endpointIDs = append(endpointIDs, e.SourceNodeID, e.TargetNodeID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, err
	}
	rows, err = tx.Query(ctx, prefix+graphNodeSelect+` WHERE i.project_id=$1 AND n.investigation_id IN (SELECT id FROM view_scope)
 AND ($4::uuid IS NULL OR n.id=ANY($5::uuid[]) OR EXISTS(SELECT 1 FROM hypothesis_nodes hn WHERE hn.hypothesis_id=$4 AND hn.investigation_id=n.investigation_id AND hn.node_id=n.id)) ORDER BY n.id`, r.ProjectID, r.InvestigationID, r.Filter.IncludeSubtree, r.HypothesisID, endpointIDs)
	if err != nil {
		return empty, err
	}
	nodes := []model.GraphNode{}
	for rows.Next() {
		n, e := scanGraphNode(rows)
		if e != nil {
			rows.Close()
			return empty, e
		}
		nodes = append(nodes, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, err
	}
	all := []model.Group{}
	objectIDs := map[string][]string{"entity": {}, "event": {}}
	viewIDs := []string{}
	for _, n := range nodes {
		viewIDs = append(viewIDs, n.InvestigationID)
		if n.EntityID != nil {
			objectIDs["entity"] = append(objectIDs["entity"], *n.EntityID)
		}
		if n.EventID != nil {
			objectIDs["event"] = append(objectIDs["event"], *n.EventID)
		}
	}
	attachments := map[string]map[string]bool{}
	for _, family := range []string{"entity", "event"} {
		attachments[family], err = projectionAttachmentsTx(ctx, tx, r.ProjectID, family, viewIDs, objectIDs[family])
		if err != nil {
			return empty, err
		}
		gs, e := listTreeGroupsTx(ctx, tx, scope, family, objectIDs[family])
		if e != nil {
			return empty, e
		}
		all = append(all, gs...)
	}
	// Exclude detached evidence and soft-deleted investigations without destroying audit.
	for gi := range all {
		g := &all[gi]
		for mi := range g.Members {
			m := &g.Members[mi]
			valid := []model.GroupAssertion{}
			for _, a := range m.Assertions {
				ok := attachments[g.Family][a.InvestigationID+":"+m.ObjectID]
				for _, id := range a.EvidenceEventIDs {
					ok = ok && attachments["event"][a.InvestigationID+":"+id]
				}
				if ok {
					valid = append(valid, a)
				}
			}
			m.Assertions = valid
		}
	}
	return grouping.Project(r, scope, nodes, edges, all), nil
}

// Two bounded membership queries replace one database round trip per assertion.
func projectionAttachmentsTx(ctx context.Context, tx pgx.Tx, project, family string, viewIDs, objectIDs []string) (map[string]bool, error) {
	_, _, objects, column, _, err := groupTables(family)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.investigation_id::text,a.`+column+`::text FROM investigation_`+objects+` a JOIN investigations i ON i.id=a.investigation_id AND i.project_id=a.project_id AND i.is_deleted=false
 WHERE a.project_id=$1 AND a.investigation_id=ANY($2::uuid[]) AND a.`+column+`=ANY($3::uuid[])`, project, viewIDs, objectIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var inv, id string
		if err = rows.Scan(&inv, &id); err != nil {
			return nil, err
		}
		out[inv+":"+id] = true
	}
	return out, rows.Err()
}
