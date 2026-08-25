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
	stats.Warnings = dedupeStrings(stats.Warnings)
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
	findingIDs := make(map[string]string, len(request.Selection.Findings))
	sessionIDs := make(map[string]string, len(request.Selection.Sessions))
	entityInput := make(map[string]struct{}, len(request.Selection.Entities))
	eventInput := make(map[string]struct{}, len(request.Selection.Events))
	findingInput := make(map[string]struct{}, len(request.Selection.Findings))
	sessionInput := make(map[string]struct{}, len(request.Selection.Sessions))
	eventSources := make(map[string]struct{}, len(request.Selection.Events))
	relationInput := make(map[string]struct{}, len(request.Selection.Relations))
	relationSources := make(map[string]struct{}, len(request.Selection.Relations))
	stats := model.ImportStats{Warnings: append([]string(nil), request.Warnings...)}

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

	for _, finding := range request.Selection.Findings {
		if _, duplicate := findingInput[finding.SnapshotID]; duplicate {
			return stats, store.ErrInvalidValue
		}
		findingInput[finding.SnapshotID] = struct{}{}
		id, err := upsertFindingTx(ctx, tx, request.ProjectID, finding)
		if err != nil {
			return stats, err
		}
		findingIDs[finding.SnapshotID] = id
		if finding.ContextStatus == "partial" {
			stats.Warnings = append(stats.Warnings, partialContextWarning(finding.Ref))
		}
	}
	for _, session := range request.Selection.Sessions {
		if _, duplicate := sessionInput[session.SnapshotID]; duplicate {
			return stats, store.ErrInvalidValue
		}
		sessionInput[session.SnapshotID] = struct{}{}
		id, err := upsertSessionTx(ctx, tx, request.ProjectID, session)
		if err != nil {
			return stats, err
		}
		sessionIDs[session.SnapshotID] = id
		if session.ContextStatus == "partial" {
			stats.Warnings = append(stats.Warnings, partialContextWarning(session.Ref))
		}
	}

	// Snapshot keys only connect normalized records inside this bounded import.
	// They are not persisted, so every referenced entity must be present here.
	for _, event := range request.Selection.Events {
		for _, mention := range event.Entities {
			if _, ok := entityIDs[mention.SnapshotID]; !ok || len(mention.Roles) == 0 {
				return stats, store.ErrUnknownReference
			}
		}
	}

	for _, finding := range request.Selection.Findings {
		findingID := findingIDs[finding.SnapshotID]
		direct, derived := finding.Direct, !finding.Direct
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_findings
			(investigation_id,finding_id,project_id,directly_added,derived)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5) ON CONFLICT DO NOTHING`,
			request.InvestigationID, findingID, request.ProjectID, direct, derived)
		if err != nil {
			return stats, fmt.Errorf("attach finding: %w", mapConstraint(err))
		}
		stats.Findings += int(tag.RowsAffected())
		if _, err := tx.Exec(ctx, `UPDATE investigation_findings
			SET directly_added=directly_added OR $4,derived=derived OR $5
			WHERE investigation_id=$1::uuid AND finding_id=$2::uuid AND project_id=$3`,
			request.InvestigationID, findingID, request.ProjectID, direct, derived); err != nil {
			return stats, fmt.Errorf("merge finding membership: %w", mapConstraint(err))
		}
	}
	for _, session := range request.Selection.Sessions {
		sessionID := sessionIDs[session.SnapshotID]
		direct, derived := session.Direct, !session.Direct
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_sessions
			(investigation_id,session_id,project_id,directly_added,derived)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5) ON CONFLICT DO NOTHING`,
			request.InvestigationID, sessionID, request.ProjectID, direct, derived)
		if err != nil {
			return stats, fmt.Errorf("attach session: %w", mapConstraint(err))
		}
		stats.Sessions += int(tag.RowsAffected())
		if _, err := tx.Exec(ctx, `UPDATE investigation_sessions
			SET directly_added=directly_added OR $4,derived=derived OR $5
			WHERE investigation_id=$1::uuid AND session_id=$2::uuid AND project_id=$3`,
			request.InvestigationID, sessionID, request.ProjectID, direct, derived); err != nil {
			return stats, fmt.Errorf("merge session membership: %w", mapConstraint(err))
		}
	}

	for _, event := range request.Selection.Events {
		eventID := eventIDs[event.SnapshotID]
		direct, derived := event.Direct, !event.Direct
		attachedBy := request.Origin
		if derived {
			attachedBy = "system"
		}
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_events
			(investigation_id,event_id,project_id,attached_by,directly_added,derived)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			request.InvestigationID, eventID, request.ProjectID, attachedBy, direct, derived)
		if err != nil {
			return stats, fmt.Errorf("attach event: %w", mapConstraint(err))
		}
		stats.Events += int(tag.RowsAffected())
		if _, err := tx.Exec(ctx, `UPDATE investigation_events
			SET directly_added=directly_added OR $4,derived=derived OR $5,
			    attached_by=CASE WHEN $4 THEN $6 ELSE attached_by END
			WHERE investigation_id=$1::uuid AND event_id=$2::uuid AND project_id=$3`,
			request.InvestigationID, eventID, request.ProjectID, direct, derived, request.Origin); err != nil {
			return stats, fmt.Errorf("merge event membership: %w", mapConstraint(err))
		}
	}
	for _, entity := range request.Selection.Entities {
		entityID := entityIDs[entity.SnapshotID]
		direct, derived := entity.Direct, !entity.Direct
		addedVia := request.Origin
		if derived {
			addedVia = "event"
		}
		tag, err := tx.Exec(ctx, `INSERT INTO investigation_entities
			(investigation_id,entity_id,project_id,added_via,directly_added,derived)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6) ON CONFLICT DO NOTHING`,
			request.InvestigationID, entityID, request.ProjectID, addedVia, direct, derived)
		if err != nil {
			return stats, fmt.Errorf("attach entity: %w", mapConstraint(err))
		}
		stats.Entities += int(tag.RowsAffected())
		if _, err := tx.Exec(ctx, `UPDATE investigation_entities
			SET directly_added=directly_added OR $4,derived=derived OR $5,
			    added_via=CASE WHEN $4 THEN $6 ELSE added_via END
			WHERE investigation_id=$1::uuid AND entity_id=$2::uuid AND project_id=$3`,
			request.InvestigationID, entityID, request.ProjectID, direct, derived, request.Origin); err != nil {
			return stats, fmt.Errorf("merge entity membership: %w", mapConstraint(err))
		}
	}

	for _, event := range request.Selection.Events {
		eventID := eventIDs[event.SnapshotID]
		for _, mention := range event.Entities {
			entityID := entityIDs[mention.SnapshotID]
			for _, role := range mention.Roles {
				role = strings.TrimSpace(role)
				if role == "" {
					return stats, store.ErrInvalidValue
				}
				if err := ensureRelationTypeTx(ctx, tx, role, "event", "entity"); err != nil {
					return stats, err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO event_entity_relations (project_id,event_id,entity_id,relation_code) VALUES ($1,$2::uuid,$3::uuid,$4) ON CONFLICT DO NOTHING`, request.ProjectID, eventID, entityID, role); err != nil {
					return stats, fmt.Errorf("link event entity: %w", mapConstraint(err))
				}
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

	findingIDsByRef := make(map[string]string, len(request.Selection.Findings))
	for _, finding := range request.Selection.Findings {
		findingID := findingIDs[finding.SnapshotID]
		findingIDsByRef[objectReferenceKey(finding.Ref)] = findingID
		for _, snapshotID := range finding.EventSnapshotIDs {
			eventID, ok := eventIDs[snapshotID]
			if !ok {
				return stats, store.ErrUnknownReference
			}
			if _, err := tx.Exec(ctx, `INSERT INTO finding_events (project_id,finding_id,event_id) VALUES ($1,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, request.ProjectID, findingID, eventID); err != nil {
				return stats, fmt.Errorf("link finding event: %w", mapConstraint(err))
			}
		}
		for _, snapshotID := range finding.EntitySnapshotIDs {
			entityID, ok := entityIDs[snapshotID]
			if !ok {
				return stats, store.ErrUnknownReference
			}
			if _, err := tx.Exec(ctx, `INSERT INTO finding_entities (project_id,finding_id,entity_id) VALUES ($1,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, request.ProjectID, findingID, entityID); err != nil {
				return stats, fmt.Errorf("link finding entity: %w", mapConstraint(err))
			}
		}
	}
	sessionIDsByRef := make(map[string]string, len(request.Selection.Sessions))
	for _, session := range request.Selection.Sessions {
		sessionID := sessionIDs[session.SnapshotID]
		sessionIDsByRef[objectReferenceKey(session.Ref)] = sessionID
		for _, snapshotID := range session.EventSnapshotIDs {
			eventID, ok := eventIDs[snapshotID]
			if !ok {
				return stats, store.ErrUnknownReference
			}
			if _, err := tx.Exec(ctx, `INSERT INTO network_session_events (project_id,session_id,event_id) VALUES ($1,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, request.ProjectID, sessionID, eventID); err != nil {
				return stats, fmt.Errorf("link session event: %w", mapConstraint(err))
			}
		}
		for _, snapshotID := range session.EntitySnapshotIDs {
			entityID, ok := entityIDs[snapshotID]
			if !ok {
				return stats, store.ErrUnknownReference
			}
			if _, err := tx.Exec(ctx, `INSERT INTO network_session_entities (project_id,session_id,entity_id) VALUES ($1,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, request.ProjectID, sessionID, entityID); err != nil {
				return stats, fmt.Errorf("link session entity: %w", mapConstraint(err))
			}
		}
	}
	for _, finding := range request.Selection.Findings {
		findingID := findingIDs[finding.SnapshotID]
		for _, related := range finding.RelatedFindings {
			if relatedID, ok := findingIDsByRef[objectReferenceKey(related)]; ok && relatedID != findingID {
				if _, err := tx.Exec(ctx, `INSERT INTO finding_relations (project_id,source_finding_id,target_finding_id,relation_code) VALUES ($1,$2::uuid,$3::uuid,'contains') ON CONFLICT DO NOTHING`, request.ProjectID, findingID, relatedID); err != nil {
					return stats, fmt.Errorf("link related finding: %w", mapConstraint(err))
				}
			}
		}
		for _, related := range finding.RelatedSessions {
			if relatedID, ok := sessionIDsByRef[objectReferenceKey(related)]; ok {
				if _, err := tx.Exec(ctx, `INSERT INTO finding_sessions (project_id,finding_id,session_id,relation_code) VALUES ($1,$2::uuid,$3::uuid,'related_session') ON CONFLICT DO NOTHING`, request.ProjectID, findingID, relatedID); err != nil {
					return stats, fmt.Errorf("link finding session: %w", mapConstraint(err))
				}
			}
		}
	}
	for _, session := range request.Selection.Sessions {
		sessionID := sessionIDs[session.SnapshotID]
		for _, related := range session.RelatedFindings {
			if findingID, ok := findingIDsByRef[objectReferenceKey(related)]; ok {
				if _, err := tx.Exec(ctx, `INSERT INTO finding_sessions (project_id,finding_id,session_id,relation_code) VALUES ($1,$2::uuid,$3::uuid,'related_session') ON CONFLICT DO NOTHING`, request.ProjectID, findingID, sessionID); err != nil {
					return stats, fmt.Errorf("link session finding: %w", mapConstraint(err))
				}
			}
		}
	}
	sourceNodes, sourceEdges, sourceWarnings, err := linkSourceSubeventEdgesTx(ctx, tx, request, eventIDs)
	if err != nil {
		return stats, err
	}
	stats.Nodes += sourceNodes
	stats.Edges += sourceEdges
	stats.Warnings = append(stats.Warnings, sourceWarnings...)

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
			for _, mention := range event.Entities {
				for _, role := range mention.Roles {
					edgeID, inserted, err := insertEdgeTx(ctx, tx, request.InvestigationID, eventNodes[event.SnapshotID], entityNodes[mention.SnapshotID], role, "confirmed", "analyst", nil, &analystConfidence, nil, nil)
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

func linkSourceSubeventEdgesTx(ctx context.Context, tx pgx.Tx, request model.ImportRequest, eventIDs map[string]string) (int, int, []string, error) {
	insertedNodes := 0
	insertedEdges := 0
	warnings := make([]string, 0)
	confidence := float32(1)
	if err := ensureRelationTypeTx(ctx, tx, "subevent_of", "event", "event"); err != nil {
		return 0, 0, nil, err
	}
	for _, child := range request.Selection.Events {
		parentSourceEventID, ok := sourceSubeventParent(child)
		if !ok {
			continue
		}
		childEventID := eventIDs[child.SnapshotID]
		if parentSourceEventID == child.Provenance.ExternalID {
			warnings = append(warnings, missingSubeventParentWarning(child.Provenance.Source))
			continue
		}
		var parentEventID string
		err := tx.QueryRow(ctx, `SELECT e.id::text
			FROM events e
			JOIN investigation_events ie
			  ON ie.event_id=e.id AND ie.project_id=e.project_id
			WHERE e.project_id=$1 AND e.source_code=$2 AND e.source_event_id=$3
			  AND ie.investigation_id=$4::uuid`, request.ProjectID, child.Provenance.Source,
			parentSourceEventID, request.InvestigationID).Scan(&parentEventID)
		if errors.Is(err, pgx.ErrNoRows) {
			warnings = append(warnings, missingSubeventParentWarning(child.Provenance.Source))
			continue
		}
		if err != nil {
			return 0, 0, nil, fmt.Errorf("resolve source subevent parent: %w", mapConstraint(err))
		}
		childNode, inserted, err := upsertNodeTx(ctx, tx, request.InvestigationID, "event", nil, &childEventID, "rule", nil)
		if err != nil {
			return 0, 0, nil, err
		}
		if inserted {
			insertedNodes++
		}
		parentNode, inserted, err := upsertNodeTx(ctx, tx, request.InvestigationID, "event", nil, &parentEventID, "rule", nil)
		if err != nil {
			return 0, 0, nil, err
		}
		if inserted {
			insertedNodes++
		}
		metadata, _ := json.Marshal(map[string]string{
			"relation_type":          "subevent_of",
			"source_code":            child.Provenance.Source,
			"child_source_event_id":  child.Provenance.ExternalID,
			"parent_source_event_id": parentSourceEventID,
		})
		edgeID, inserted, err := insertEdgeTx(ctx, tx, request.InvestigationID, childNode, parentNode,
			"subevent_of", "confirmed", "rule", child.Provenance.SourceURL, &confidence, nil, metadata)
		if err != nil {
			return 0, 0, nil, err
		}
		if inserted {
			insertedEdges++
		}
		if _, err := tx.Exec(ctx, `INSERT INTO edge_evidence (edge_id,event_id,investigation_id)
			VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, edgeID, childEventID, request.InvestigationID); err != nil {
			return 0, 0, nil, fmt.Errorf("add source subevent evidence: %w", mapConstraint(err))
		}
	}
	return insertedNodes, insertedEdges, warnings, nil
}

func sourceSubeventParent(event model.GatewayEvent) (string, bool) {
	relationType, relationOK := event.Attributes["relation_type"].(string)
	parentID, parentOK := event.Attributes["parent_source_event_id"].(string)
	parentID = strings.TrimSpace(parentID)
	return parentID, relationOK && parentOK && relationType == "subevent_of" && parentID != ""
}

func missingSubeventParentWarning(source string) string {
	return "source relation subevent_of skipped: parent event is unavailable in this investigation for " + strings.TrimSpace(source)
}

func ensureSourceTx(ctx context.Context, tx pgx.Tx, code string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sources WHERE code=$1)`, strings.TrimSpace(code)).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check source: %w", mapConstraint(err))
	}
	if !exists {
		return store.ErrInvalidValue
	}
	return nil
}

func upsertFindingTx(ctx context.Context, tx pgx.Tx, projectID string, finding model.GatewayFinding) (string, error) {
	if strings.TrimSpace(finding.SnapshotID) == "" || strings.TrimSpace(finding.Title) == "" ||
		strings.TrimSpace(finding.Kind) == "" || finding.OccurredAt.IsZero() || finding.FetchedAt.IsZero() ||
		finding.Kind != finding.Ref.RecordType || !validObjectRef(finding.Ref) ||
		(finding.ContextStatus != "complete" && finding.ContextStatus != "partial") {
		return "", store.ErrInvalidValue
	}
	if err := ensureSourceTx(ctx, tx, finding.Ref.SourceCode); err != nil {
		return "", err
	}
	normalized := finding.Normalized
	if len(normalized) == 0 {
		normalized = []byte(`{}`)
	}
	provenance := finding.Provenance
	if len(provenance) == 0 {
		provenance, _ = json.Marshal(map[string]any{"ref": finding.Ref, "source_ref": finding.SourceRef, "fetched_at": finding.FetchedAt})
	}
	contextErrors, err := json.Marshal(append([]model.GatewayContextError{}, finding.ContextErrors...))
	if err != nil {
		return "", store.ErrInvalidValue
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO findings
		(project_id,source_code,source_instance,record_type,external_id,time_from,time_to,
		 kind,title,description,severity,occurred_at,status,source_ref,fetched_at,
		 normalized_snapshot,provenance,context_status,context_errors)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb,$17::jsonb,$18,$19::jsonb)
		ON CONFLICT (project_id,source_code,source_instance,record_type,external_id) DO UPDATE SET
		 time_from=EXCLUDED.time_from,time_to=EXCLUDED.time_to,kind=EXCLUDED.kind,title=EXCLUDED.title,
		 description=EXCLUDED.description,severity=EXCLUDED.severity,occurred_at=EXCLUDED.occurred_at,
		 status=EXCLUDED.status,source_ref=EXCLUDED.source_ref,fetched_at=EXCLUDED.fetched_at,
		 normalized_snapshot=CASE WHEN EXCLUDED.context_status='partial' AND findings.context_status='complete'
		   THEN findings.normalized_snapshot || EXCLUDED.normalized_snapshot ELSE EXCLUDED.normalized_snapshot END,
		 provenance=findings.provenance || EXCLUDED.provenance,
		 context_status=EXCLUDED.context_status,context_errors=EXCLUDED.context_errors
		RETURNING id::text`, projectID, finding.Ref.SourceCode, finding.Ref.SourceInstance,
		finding.Ref.RecordType, finding.Ref.ExternalID, finding.Ref.TimeRange.From, finding.Ref.TimeRange.To,
		finding.Kind, finding.Title, finding.Description, finding.Severity, finding.OccurredAt, finding.Status,
		finding.SourceRef, finding.FetchedAt, string(normalized), string(provenance), finding.ContextStatus,
		string(contextErrors)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert finding: %w", mapConstraint(err))
	}
	return id, nil
}

func upsertSessionTx(ctx context.Context, tx pgx.Tx, projectID string, session model.GatewaySession) (string, error) {
	if strings.TrimSpace(session.SnapshotID) == "" || strings.TrimSpace(session.Title) == "" ||
		session.StartedAt.IsZero() || session.FetchedAt.IsZero() || session.Ref.RecordType != "nad_session" ||
		!validObjectRef(session.Ref) || (session.EndedAt != nil && session.EndedAt.Before(session.StartedAt)) ||
		(session.ContextStatus != "complete" && session.ContextStatus != "partial") {
		return "", store.ErrInvalidValue
	}
	if err := ensureSourceTx(ctx, tx, session.Ref.SourceCode); err != nil {
		return "", err
	}
	normalized := session.Normalized
	if len(normalized) == 0 {
		normalized = []byte(`{}`)
	}
	provenance := session.Provenance
	if len(provenance) == 0 {
		provenance, _ = json.Marshal(map[string]any{"ref": session.Ref, "source_ref": session.SourceRef, "fetched_at": session.FetchedAt})
	}
	contextErrors, err := json.Marshal(append([]model.GatewayContextError{}, session.ContextErrors...))
	if err != nil {
		return "", store.ErrInvalidValue
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO network_sessions
		(project_id,source_code,source_instance,record_type,external_id,time_from,time_to,
		 title,severity,started_at,ended_at,source_ref,fetched_at,normalized_snapshot,provenance,context_status,context_errors)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb,$16,$17::jsonb)
		ON CONFLICT (project_id,source_code,source_instance,record_type,external_id) DO UPDATE SET
		 time_from=EXCLUDED.time_from,time_to=EXCLUDED.time_to,title=EXCLUDED.title,severity=EXCLUDED.severity,
		 started_at=EXCLUDED.started_at,ended_at=EXCLUDED.ended_at,source_ref=EXCLUDED.source_ref,
		 fetched_at=EXCLUDED.fetched_at,
		 normalized_snapshot=CASE WHEN EXCLUDED.context_status='partial' AND network_sessions.context_status='complete'
		   THEN network_sessions.normalized_snapshot || EXCLUDED.normalized_snapshot ELSE EXCLUDED.normalized_snapshot END,
		 provenance=network_sessions.provenance || EXCLUDED.provenance,
		 context_status=EXCLUDED.context_status,context_errors=EXCLUDED.context_errors
		RETURNING id::text`, projectID, session.Ref.SourceCode, session.Ref.SourceInstance,
		session.Ref.RecordType, session.Ref.ExternalID, session.Ref.TimeRange.From, session.Ref.TimeRange.To,
		session.Title, session.Severity, session.StartedAt, session.EndedAt, session.SourceRef,
		session.FetchedAt, string(normalized), string(provenance), session.ContextStatus,
		string(contextErrors)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert network session: %w", mapConstraint(err))
	}
	return id, nil
}

func validObjectRef(ref model.GatewayObjectRef) bool {
	if strings.TrimSpace(ref.SourceCode) == "" || strings.TrimSpace(ref.RecordType) == "" ||
		strings.TrimSpace(ref.ExternalID) == "" || ref.TimeRange.From.IsZero() || ref.TimeRange.To.IsZero() ||
		!ref.TimeRange.From.Before(ref.TimeRange.To) {
		return false
	}
	switch ref.SourceCode {
	case "pt-maxpatrol-siem":
		return ref.SourceInstance == "" && (ref.RecordType == "siem_incident" || ref.RecordType == "siem_correlation")
	case "pt-nad":
		return strings.TrimSpace(ref.SourceInstance) != "" && (ref.RecordType == "nad_attack" || ref.RecordType == "nad_session")
	default:
		return false
	}
}

func objectReferenceKey(ref model.GatewayObjectRef) string {
	return strings.Join([]string{strings.TrimSpace(ref.SourceCode), strings.TrimSpace(ref.SourceInstance), strings.TrimSpace(ref.RecordType), strings.TrimSpace(ref.ExternalID)}, "\x00")
}

func partialContextWarning(ref model.GatewayObjectRef) string {
	return fmt.Sprintf("partial context for %s/%s/%s", ref.SourceCode, ref.RecordType, ref.ExternalID)
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
	var err error
	var directed bool
	source, target, directed, err = normalizeEdgeNodesTx(ctx, tx, source, target, relationCode)
	if err != nil {
		return "", false, err
	}
	if !directed {
		var existingID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM edges
			WHERE investigation_id=$1::uuid AND relation_code=$2
			  AND ((source_node_id=$3::uuid AND target_node_id=$4::uuid)
			    OR (source_node_id=$4::uuid AND target_node_id=$3::uuid))
			LIMIT 1`, investigationID, relationCode, source.ID, target.ID).Scan(&existingID)
		if err == nil {
			return existingID, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", false, fmt.Errorf("find undirected edge: %w", mapConstraint(err))
		}
	}
	metadataJSON := "{}"
	if len(metadata) > 0 {
		metadataJSON = string(metadata)
	}
	var edgeID string
	err = tx.QueryRow(ctx, `INSERT INTO edges (investigation_id,source_node_id,target_node_id,relation_code,status,confidence,why,origin,origin_ref,metadata) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10::jsonb) ON CONFLICT (investigation_id,source_node_id,target_node_id,relation_code) DO NOTHING RETURNING id::text`, investigationID, source.ID, target.ID, relationCode, status, confidence, why, origin, originRef, metadataJSON).Scan(&edgeID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id::text FROM edges WHERE investigation_id=$1::uuid AND source_node_id=$2::uuid AND target_node_id=$3::uuid AND relation_code=$4`, investigationID, source.ID, target.ID, relationCode).Scan(&edgeID)
		return edgeID, false, err
	}
	if err != nil {
		return "", false, fmt.Errorf("insert edge: %w", mapConstraint(err))
	}
	return edgeID, true, nil
}
