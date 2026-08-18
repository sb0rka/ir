package psql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

type resolvedNode struct {
	Node     model.GraphNode
	EventID  *string
	NodeType string
}

func (d *DB) ImportContext(ctx context.Context, request model.ImportRequest) (model.ImportStats, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return model.ImportStats{}, fmt.Errorf("begin context import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := investigationExistsTx(ctx, tx, request.ProjectID, request.InvestigationID)
	if err != nil {
		return model.ImportStats{}, err
	}
	if !exists {
		return model.ImportStats{}, store.ErrInvestigationNotFound
	}
	stats, err := importSelectionTx(ctx, tx, request)
	if err != nil {
		return model.ImportStats{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ImportStats{}, fmt.Errorf("commit context import: %w", err)
	}
	return stats, nil
}

func importSelectionTx(ctx context.Context, tx pgx.Tx, request model.ImportRequest) (model.ImportStats, error) {
	if request.Origin != "analyst" && request.Origin != "agent" {
		return model.ImportStats{}, store.ErrInvalidValue
	}
	if request.Origin == "agent" && len(request.SomIssueIDs) == 0 {
		return model.ImportStats{}, store.ErrInvalidValue
	}

	entityIDs := make(map[string]string, len(request.Selection.Entities))
	eventIDs := make(map[string]string, len(request.Selection.Events))
	entityInput := make(map[string]struct{}, len(request.Selection.Entities))
	eventInput := make(map[string]struct{}, len(request.Selection.Events))
	eventSources := make(map[string]struct{}, len(request.Selection.Events))
	relationInput := make(map[string]struct{}, len(request.Selection.Relations))
	relationSources := make(map[string]struct{}, len(request.Selection.Relations))
	var stats model.ImportStats

	for _, entity := range request.Selection.Entities {
		if strings.TrimSpace(entity.SnapshotID) == "" || strings.TrimSpace(entity.TypeCode) == "" || strings.TrimSpace(entity.Value) == "" {
			return stats, store.ErrInvalidValue
		}
		if _, duplicate := entityInput[entity.SnapshotID]; duplicate {
			return stats, store.ErrInvalidValue
		}
		entityInput[entity.SnapshotID] = struct{}{}
		if err := ensureEntityTypeTx(ctx, tx, entity.TypeCode); err != nil {
			return stats, err
		}
		metadata, _ := json.Marshal(entity.Attributes)
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO entities (project_id,type_code,canonical_key,display_name,metadata)
			VALUES ($1,$2,$3,$3,$4::jsonb)
			ON CONFLICT (project_id,type_code,canonical_key) DO UPDATE SET
			  metadata=entities.metadata || EXCLUDED.metadata
			RETURNING id::text`, request.ProjectID, entity.TypeCode, entity.Value, string(metadata)).Scan(&id)
		if err != nil {
			return stats, fmt.Errorf("upsert entity: %w", mapConstraint(err))
		}
		entityIDs[entity.SnapshotID] = id
		for _, source := range entity.Provenance {
			if strings.TrimSpace(source.Source) == "" || strings.TrimSpace(source.ExternalID) == "" {
				return stats, store.ErrInvalidValue
			}
			if err := ensureSourceTx(ctx, tx, source.Source); err != nil {
				return stats, err
			}
			provenance, _ := json.Marshal(source)
			var mappedEntityID string
			err := tx.QueryRow(ctx, `
				INSERT INTO entity_sources (entity_id,project_id,source_code,source_entity_id,source_ref,fetched_at,provenance)
				VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)
				ON CONFLICT (project_id,source_code,source_entity_id) DO UPDATE SET
				  source_ref=EXCLUDED.source_ref,fetched_at=EXCLUDED.fetched_at,provenance=EXCLUDED.provenance
				  WHERE entity_sources.entity_id=EXCLUDED.entity_id
				RETURNING entity_id::text`, id, request.ProjectID, source.Source, source.ExternalID,
				source.SourceURL, source.FetchedAt, string(provenance)).Scan(&mappedEntityID)
			if errors.Is(err, pgx.ErrNoRows) {
				return stats, store.ErrInvalidValue
			}
			if err != nil {
				return stats, fmt.Errorf("upsert entity source: %w", mapConstraint(err))
			}
		}
	}

	for _, event := range request.Selection.Events {
		if strings.TrimSpace(event.SnapshotID) == "" || strings.TrimSpace(event.Title) == "" || strings.TrimSpace(event.EventType) == "" || strings.TrimSpace(event.Provenance.Source) == "" || strings.TrimSpace(event.Provenance.ExternalID) == "" {
			return stats, store.ErrInvalidValue
		}
		if _, duplicate := eventInput[event.SnapshotID]; duplicate {
			return stats, store.ErrInvalidValue
		}
		eventInput[event.SnapshotID] = struct{}{}
		sourceIdentity := event.Provenance.Source + "\x00" + event.Provenance.ExternalID
		if _, duplicate := eventSources[sourceIdentity]; duplicate {
			return stats, store.ErrInvalidValue
		}
		eventSources[sourceIdentity] = struct{}{}
		if err := ensureSourceTx(ctx, tx, event.Provenance.Source); err != nil {
			return stats, err
		}
		normalized, _ := json.Marshal(map[string]any{
			"source_code": event.Provenance.Source, "source_event_id": event.Provenance.ExternalID,
			"type": event.EventType, "title": event.Title,
			"severity": event.Severity, "occurred_at": event.OccurredAt,
			"attributes": event.Attributes,
		})
		provenance, _ := json.Marshal(event.Provenance)
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO events (project_id,source_code,source_event_id,source_ref,title,event_type,occurred_at,normalized_data,provenance)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb)
			ON CONFLICT (project_id,source_code,source_event_id) DO UPDATE SET
			  source_ref=EXCLUDED.source_ref,title=EXCLUDED.title,event_type=EXCLUDED.event_type,
			  occurred_at=EXCLUDED.occurred_at,normalized_data=EXCLUDED.normalized_data,provenance=EXCLUDED.provenance
			RETURNING id::text`, request.ProjectID, event.Provenance.Source, event.Provenance.ExternalID,
			event.Provenance.SourceURL, event.Title, event.EventType, event.OccurredAt,
			string(normalized), string(provenance)).Scan(&id)
		if err != nil {
			return stats, fmt.Errorf("upsert source event: %w", mapConstraint(err))
		}
		eventIDs[event.SnapshotID] = id
	}

	// Snapshot keys only connect normalized records inside this bounded import.
	// They are not persisted, so every referenced entity must be present here.
	for _, event := range request.Selection.Events {
		for _, entitySnapshotID := range event.EntitySnapshotIDs {
			if _, ok := entityIDs[entitySnapshotID]; !ok {
				return stats, store.ErrUnknownReference
			}
		}
	}

	attachedBy := request.Origin
	addedVia := request.Origin
	for _, eventID := range eventIDs {
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_events (investigation_id,event_id,project_id,attached_by) VALUES ($1::uuid,$2::uuid,$3,$4) ON CONFLICT DO NOTHING`, request.InvestigationID, eventID, request.ProjectID, attachedBy)
		if err != nil {
			return stats, fmt.Errorf("attach event: %w", mapConstraint(err))
		}
		stats.Events += int(tag.RowsAffected())
	}
	for _, entityID := range entityIDs {
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_entities (investigation_id,entity_id,project_id,added_via) VALUES ($1::uuid,$2::uuid,$3,$4) ON CONFLICT DO NOTHING`, request.InvestigationID, entityID, request.ProjectID, addedVia)
		if err != nil {
			return stats, fmt.Errorf("attach entity: %w", mapConstraint(err))
		}
		stats.Entities += int(tag.RowsAffected())
	}

	if err := ensureRelationTypeTx(ctx, tx, "mentions", "event", "entity"); err != nil {
		return stats, err
	}
	for _, event := range request.Selection.Events {
		eventID := eventIDs[event.SnapshotID]
		for _, entitySnapshotID := range event.EntitySnapshotIDs {
			entityID := entityIDs[entitySnapshotID]
			if _, err := tx.Exec(ctx, `INSERT INTO event_entity_relations (project_id,event_id,entity_id,relation_code) VALUES ($1,$2::uuid,$3::uuid,'mentions') ON CONFLICT DO NOTHING`, request.ProjectID, eventID, entityID); err != nil {
				return stats, fmt.Errorf("link event entity: %w", mapConstraint(err))
			}
			if _, err := tx.Exec(ctx, `UPDATE entities SET first_seen=LEAST(COALESCE(first_seen,$1),$1),last_seen=GREATEST(COALESCE(last_seen,$1),$1) WHERE id=$2::uuid AND project_id=$3`, event.OccurredAt, entityID, request.ProjectID); err != nil {
				return stats, err
			}
		}
	}

	for _, relation := range request.Selection.Relations {
		sourceID, sourceOK := entityIDs[relation.SourceEntitySnapshotID]
		targetID, targetOK := entityIDs[relation.TargetEntitySnapshotID]
		if !sourceOK || !targetOK || strings.TrimSpace(relation.SnapshotID) == "" || strings.TrimSpace(relation.RelationCode) == "" || strings.TrimSpace(relation.Provenance.Source) == "" || strings.TrimSpace(relation.Provenance.ExternalID) == "" {
			return stats, store.ErrUnknownReference
		}
		if _, duplicate := relationInput[relation.SnapshotID]; duplicate {
			return stats, store.ErrInvalidValue
		}
		relationInput[relation.SnapshotID] = struct{}{}
		sourceIdentity := relation.Provenance.Source + "\x00" + relation.Provenance.ExternalID
		if _, duplicate := relationSources[sourceIdentity]; duplicate {
			return stats, store.ErrInvalidValue
		}
		relationSources[sourceIdentity] = struct{}{}
		if err := ensureSourceTx(ctx, tx, relation.Provenance.Source); err != nil {
			return stats, err
		}
		if err := ensureRelationTypeTx(ctx, tx, relation.RelationCode, "entity", "entity"); err != nil {
			return stats, err
		}
		provenance, _ := json.Marshal(relation.Provenance)
		_, err := tx.Exec(ctx, `
			INSERT INTO entity_relations (project_id,source_code,source_relation_id,source_ref,source_entity_id,target_entity_id,relation_code,occurred_at,provenance)
			VALUES ($1,$2,$3,$4,$5::uuid,$6::uuid,$7,$8,$9::jsonb)
			ON CONFLICT (project_id,source_code,source_relation_id) DO UPDATE SET relation_code=EXCLUDED.relation_code,
			  source_entity_id=EXCLUDED.source_entity_id,target_entity_id=EXCLUDED.target_entity_id,
			  source_ref=EXCLUDED.source_ref,occurred_at=EXCLUDED.occurred_at,provenance=EXCLUDED.provenance`, request.ProjectID,
			relation.Provenance.Source, relation.Provenance.ExternalID, relation.Provenance.SourceURL,
			sourceID, targetID, relation.RelationCode, relation.OccurredAt, string(provenance))
		if err != nil {
			return stats, fmt.Errorf("upsert source relation: %w", mapConstraint(err))
		}
	}

	if request.Origin == "analyst" {
		analystConfidence := float32(1)
		eventNodes := make(map[string]model.GraphNode, len(eventIDs))
		entityNodes := make(map[string]model.GraphNode, len(entityIDs))
		for snapshotID, eventID := range eventIDs {
			node, inserted, err := upsertNodeTx(ctx, tx, request.InvestigationID, "event", nil, &eventID, "analyst", nil)
			if err != nil {
				return stats, err
			}
			eventNodes[snapshotID] = node
			if inserted {
				stats.Nodes++
			}
		}
		for snapshotID, entityID := range entityIDs {
			node, inserted, err := upsertNodeTx(ctx, tx, request.InvestigationID, "entity", &entityID, nil, "analyst", nil)
			if err != nil {
				return stats, err
			}
			entityNodes[snapshotID] = node
			if inserted {
				stats.Nodes++
			}
		}
		for _, event := range request.Selection.Events {
			for _, entitySnapshotID := range event.EntitySnapshotIDs {
				edgeID, inserted, err := insertEdgeTx(ctx, tx, request.InvestigationID, eventNodes[event.SnapshotID], entityNodes[entitySnapshotID], "mentions", "confirmed", "analyst", nil, &analystConfidence, nil, nil)
				if err != nil {
					return stats, err
				}
				if inserted {
					stats.Edges++
				}
				if _, err := tx.Exec(ctx, `INSERT INTO edge_evidence (edge_id,event_id,investigation_id) VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, edgeID, eventIDs[event.SnapshotID], request.InvestigationID); err != nil {
					return stats, err
				}
			}
		}
		for _, relation := range request.Selection.Relations {
			metadata, _ := json.Marshal(map[string]any{"source_code": relation.Provenance.Source, "source_relation_id": relation.Provenance.ExternalID, "source_ref": relation.Provenance.SourceURL})
			_, inserted, err := insertEdgeTx(ctx, tx, request.InvestigationID, entityNodes[relation.SourceEntitySnapshotID], entityNodes[relation.TargetEntitySnapshotID], relation.RelationCode, "confirmed", "analyst", nil, &analystConfidence, nil, metadata)
			if err != nil {
				return stats, err
			}
			if inserted {
				stats.Edges++
			}
		}
		return stats, nil
	}

	local := make(map[string]resolvedNode, len(request.Nodes))
	for _, spec := range request.Nodes {
		ref := strings.TrimSpace(spec.Ref)
		if ref == "" {
			return stats, store.ErrInvalidValue
		}
		if _, exists := local[ref]; exists {
			return stats, store.ErrInvalidValue
		}
		targets := 0
		if spec.SnapshotEventID != nil {
			targets++
		}
		if spec.SnapshotEntityID != nil {
			targets++
		}
		if spec.NodeID != nil {
			targets++
		}
		if targets != 1 {
			return stats, store.ErrInvalidValue
		}
		var node model.GraphNode
		var inserted bool
		var err error
		if spec.SnapshotEventID != nil {
			if _, supplied := eventInput[*spec.SnapshotEventID]; !supplied {
				return stats, store.ErrUnknownReference
			}
			eventID := eventIDs[*spec.SnapshotEventID]
			node, inserted, err = upsertNodeTx(ctx, tx, request.InvestigationID, "event", nil, &eventID, "agent", request.SomIssueIDs)
		} else if spec.SnapshotEntityID != nil {
			if _, supplied := entityInput[*spec.SnapshotEntityID]; !supplied {
				return stats, store.ErrUnknownReference
			}
			entityID := entityIDs[*spec.SnapshotEntityID]
			node, inserted, err = upsertNodeTx(ctx, tx, request.InvestigationID, "entity", &entityID, nil, "agent", request.SomIssueIDs)
		} else {
			node, err = graphNodeByIDTx(ctx, tx, request.ProjectID, request.InvestigationID, *spec.NodeID)
			if err == nil {
				err = linkNodeIssuesTx(ctx, tx, node.ID, request.SomIssueIDs)
			}
		}
		if err != nil {
			return stats, err
		}
		if inserted {
			stats.Nodes++
		}
		local[ref] = resolvedNode{Node: node, EventID: node.EventID, NodeType: node.NodeType}
	}

	originRef := request.SomIssueIDs[0]
	for _, spec := range request.Edges {
		source, sourceOK := local[spec.SourceRef]
		target, targetOK := local[spec.TargetRef]
		if !sourceOK || !targetOK || strings.TrimSpace(spec.Why) == "" || len(spec.EvidenceEventRefs) == 0 {
			return stats, store.ErrUnknownReference
		}
		if err := ensureRelationTypeTx(ctx, tx, spec.RelationCode, source.NodeType, target.NodeType); err != nil {
			return stats, err
		}
		why := strings.TrimSpace(spec.Why)
		edgeID, inserted, err := insertEdgeTx(ctx, tx, request.InvestigationID, source.Node, target.Node, spec.RelationCode, "proposed", "agent", &originRef, spec.Confidence, &why, nil)
		if err != nil {
			return stats, err
		}
		for _, evidenceRef := range spec.EvidenceEventRefs {
			evidence, ok := local[evidenceRef]
			if !ok || evidence.EventID == nil {
				return stats, store.ErrUnknownReference
			}
			if _, err := tx.Exec(ctx, `INSERT INTO edge_evidence (edge_id,event_id,investigation_id) VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, edgeID, *evidence.EventID, request.InvestigationID); err != nil {
				return stats, fmt.Errorf("add edge evidence: %w", mapConstraint(err))
			}
		}
		if inserted {
			stats.Edges++
		}
	}
	return stats, nil
}

func ensureSourceTx(ctx context.Context, tx pgx.Tx, code string) error {
	_, err := tx.Exec(ctx, `INSERT INTO sources (code,kind,title) VALUES ($1::varchar,'other','Gateway: '||$1::text) ON CONFLICT (code) DO NOTHING`, code)
	if err != nil {
		return fmt.Errorf("ensure source: %w", mapConstraint(err))
	}
	return nil
}

func ensureEntityTypeTx(ctx context.Context, tx pgx.Tx, code string) error {
	_, err := tx.Exec(ctx, `INSERT INTO entity_types (code,title,category) VALUES ($1::varchar,'Gateway: '||$1::text,'other') ON CONFLICT (code) DO NOTHING`, code)
	if err != nil {
		return fmt.Errorf("ensure entity type: %w", mapConstraint(err))
	}
	return nil
}

func ensureRelationTypeTx(ctx context.Context, tx pgx.Tx, code, sourceKind, targetKind string) error {
	_, err := tx.Exec(ctx, `INSERT INTO relation_types (code,title,source_kind,target_kind,directed) VALUES ($1::varchar,'Gateway: '||$1::text,$2,$3,true) ON CONFLICT (code) DO NOTHING`, code, sourceKind, targetKind)
	if err != nil {
		return fmt.Errorf("ensure relation type: %w", mapConstraint(err))
	}
	var actualSource, actualTarget string
	if err := tx.QueryRow(ctx, `SELECT source_kind,target_kind FROM relation_types WHERE code=$1`, code).Scan(&actualSource, &actualTarget); err != nil {
		return err
	}
	if actualSource != sourceKind || actualTarget != targetKind {
		return store.ErrInvalidValue
	}
	return nil
}

func upsertNodeTx(ctx context.Context, tx pgx.Tx, investigationID, nodeType string, entityID, eventID *string, origin string, somIssueIDs []string) (model.GraphNode, bool, error) {
	var nodeID string
	err := tx.QueryRow(ctx, `INSERT INTO graph_nodes (investigation_id,node_type,entity_id,event_id,origin) VALUES ($1::uuid,$2,$3::uuid,$4::uuid,$5) ON CONFLICT DO NOTHING RETURNING id::text`, investigationID, nodeType, entityID, eventID, origin).Scan(&nodeID)
	inserted := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text FROM graph_nodes WHERE investigation_id=$1::uuid AND (($2::uuid IS NOT NULL AND entity_id=$2::uuid) OR ($3::uuid IS NOT NULL AND event_id=$3::uuid))`, investigationID, entityID, eventID).Scan(&nodeID)
	}
	if err != nil {
		return model.GraphNode{}, false, fmt.Errorf("upsert graph node: %w", mapConstraint(err))
	}
	if err := linkNodeIssuesTx(ctx, tx, nodeID, somIssueIDs); err != nil {
		return model.GraphNode{}, false, err
	}
	node, err := graphNodeByIDTx(ctx, tx, "", investigationID, nodeID)
	return node, inserted, err
}

func linkNodeIssuesTx(ctx context.Context, tx pgx.Tx, nodeID string, somIssueIDs []string) error {
	for _, issueID := range somIssueIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO graph_node_som_issues (graph_node_id,som_issue_id) VALUES ($1::uuid,$2::uuid) ON CONFLICT DO NOTHING`, nodeID, issueID); err != nil {
			return fmt.Errorf("link SOM issue: %w", mapConstraint(err))
		}
	}
	return nil
}

func insertEdgeTx(ctx context.Context, tx pgx.Tx, investigationID string, source, target model.GraphNode, relationCode, status, origin string, originRef *string, confidence *float32, why *string, metadata []byte) (string, bool, error) {
	if source.InvestigationID != investigationID || target.InvestigationID != investigationID {
		return "", false, store.ErrUnknownReference
	}
	metadataJSON := "{}"
	if len(metadata) > 0 {
		metadataJSON = string(metadata)
	}
	var edgeID string
	err := tx.QueryRow(ctx, `INSERT INTO edges (investigation_id,source_node_id,target_node_id,relation_code,status,confidence,why,origin,origin_ref,metadata) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10::jsonb) ON CONFLICT (investigation_id,source_node_id,target_node_id,relation_code) DO NOTHING RETURNING id::text`, investigationID, source.ID, target.ID, relationCode, status, confidence, why, origin, originRef, metadataJSON).Scan(&edgeID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text FROM edges WHERE investigation_id=$1::uuid AND source_node_id=$2::uuid AND target_node_id=$3::uuid AND relation_code=$4`, investigationID, source.ID, target.ID, relationCode).Scan(&edgeID)
		return edgeID, false, err
	}
	if err != nil {
		return "", false, fmt.Errorf("insert edge: %w", mapConstraint(err))
	}
	return edgeID, true, nil
}
