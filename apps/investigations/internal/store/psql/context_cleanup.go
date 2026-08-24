package psql

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func cleanupDerivedMembershipsTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string) error {
	// Repeat until the coarse-object ownership graph reaches a fixed point.
	for {
		findingTag, err := tx.Exec(ctx, `DELETE FROM investigation_findings child
			WHERE child.investigation_id=$1::uuid AND child.project_id=$2
			  AND child.derived AND NOT child.directly_added
			  AND NOT EXISTS (
			    SELECT 1 FROM finding_relations r
			    JOIN investigation_findings parent
			      ON parent.finding_id=r.source_finding_id
			     AND parent.investigation_id=child.investigation_id
			     AND parent.project_id=child.project_id
			    WHERE r.target_finding_id=child.finding_id AND r.project_id=child.project_id)`, investigationID, projectID)
		if err != nil {
			return fmt.Errorf("remove unowned derived findings: %w", mapConstraint(err))
		}
		sessionTag, err := tx.Exec(ctx, `DELETE FROM investigation_sessions child
			WHERE child.investigation_id=$1::uuid AND child.project_id=$2
			  AND child.derived AND NOT child.directly_added
			  AND NOT EXISTS (
			    SELECT 1 FROM finding_sessions r
			    JOIN investigation_findings parent
			      ON parent.finding_id=r.finding_id
			     AND parent.investigation_id=child.investigation_id
			     AND parent.project_id=child.project_id
			    WHERE r.session_id=child.session_id AND r.project_id=child.project_id)`, investigationID, projectID)
		if err != nil {
			return fmt.Errorf("remove unowned derived sessions: %w", mapConstraint(err))
		}
		if findingTag.RowsAffected() == 0 && sessionTag.RowsAffected() == 0 {
			return nil
		}
	}
}

func cleanupExclusiveGranularContextTx(ctx context.Context, tx pgx.Tx, projectID, investigationID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM investigation_events ie
		WHERE ie.investigation_id=$1::uuid AND ie.project_id=$2
		  AND ie.derived AND NOT ie.directly_added
		  AND NOT EXISTS (
		    SELECT 1 FROM finding_events fe JOIN investigation_findings f
		      ON f.finding_id=fe.finding_id AND f.investigation_id=ie.investigation_id AND f.project_id=ie.project_id
		    WHERE fe.event_id=ie.event_id AND fe.project_id=ie.project_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM network_session_events se JOIN investigation_sessions s
		      ON s.session_id=se.session_id AND s.investigation_id=ie.investigation_id AND s.project_id=ie.project_id
		    WHERE se.event_id=ie.event_id AND se.project_id=ie.project_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM edge_evidence ee JOIN edges e ON e.id=ee.edge_id
		    WHERE ee.investigation_id=ie.investigation_id AND ee.event_id=ie.event_id AND e.status='confirmed')
		  AND NOT EXISTS (
		    SELECT 1 FROM graph_nodes n JOIN edges e
		      ON e.investigation_id=n.investigation_id
		     AND (e.source_node_id=n.id OR e.target_node_id=n.id)
		    WHERE n.investigation_id=ie.investigation_id AND n.event_id=ie.event_id AND e.status='confirmed')`, investigationID, projectID); err != nil {
		return fmt.Errorf("remove exclusively derived events: %w", mapConstraint(err))
	}

	if _, err := tx.Exec(ctx, `DELETE FROM investigation_entities ie
		WHERE ie.investigation_id=$1::uuid AND ie.project_id=$2
		  AND ie.derived AND NOT ie.directly_added
		  AND NOT EXISTS (
		    SELECT 1 FROM finding_entities fe JOIN investigation_findings f
		      ON f.finding_id=fe.finding_id AND f.investigation_id=ie.investigation_id AND f.project_id=ie.project_id
		    WHERE fe.entity_id=ie.entity_id AND fe.project_id=ie.project_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM network_session_entities se JOIN investigation_sessions s
		      ON s.session_id=se.session_id AND s.investigation_id=ie.investigation_id AND s.project_id=ie.project_id
		    WHERE se.entity_id=ie.entity_id AND se.project_id=ie.project_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM event_entity_relations r JOIN investigation_events ev
		      ON ev.event_id=r.event_id AND ev.investigation_id=ie.investigation_id AND ev.project_id=ie.project_id
		    WHERE r.entity_id=ie.entity_id AND r.project_id=ie.project_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM graph_nodes n JOIN edges e
		      ON e.investigation_id=n.investigation_id
		     AND (e.source_node_id=n.id OR e.target_node_id=n.id)
		    WHERE n.investigation_id=ie.investigation_id AND n.entity_id=ie.entity_id AND e.status='confirmed')`, investigationID, projectID); err != nil {
		return fmt.Errorf("remove exclusively derived entities: %w", mapConstraint(err))
	}
	return nil
}
