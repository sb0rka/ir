package psql

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/grouping"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

func newSourceGroup(scope model.GroupScope, family, kind, typ, key, title string) model.Group {
	if len([]rune(title)) > 255 {
		title = string([]rune(title)[:255])
	}
	return model.Group{ID: uuid.NewString(), GroupScope: scope, Family: family, Kind: kind, TypeCode: typ, Key: key, Title: title, State: "active", Version: 1, Members: []model.GroupMember{}, SuccessorIDs: []string{}}
}

func sourceAssertion(inv, ref, method string, evidence []string) model.GroupAssertion {
	evidence = slices.Clone(evidence)
	slices.Sort(evidence)
	evidence = slices.Compact(evidence)
	if evidence == nil {
		evidence = []string{}
	}
	return model.GroupAssertion{InvestigationID: inv, Origin: "source", OriginRef: ref, Method: method, MethodVersion: "1", EvidenceEventIDs: evidence, Reason: "Explicit source relationship"}
}

func sourceMember(object, role string, a model.GroupAssertion) model.GroupMember {
	confidence := float32(1)
	return model.GroupMember{ID: uuid.NewString(), ObjectID: object, Role: role, Status: "confirmed", Confidence: &confidence, Reason: a.Reason, Version: 1, Assertions: []model.GroupAssertion{a}}
}

// Source identity tuples contain NUL separators in memory; PostgreSQL text/jsonb
// must receive a printable, unambiguous encoding instead.
func sourceGroupObjectKey(ref model.GatewayObjectRef) string {
	return base64.RawURLEncoding.EncodeToString([]byte(objectReferenceKey(ref)))
}

func sourceCompositeKey(source, parent string) string {
	return "source-composite:v1:" + base64.RawURLEncoding.EncodeToString([]byte(source+"\x00"+parent))
}

func importGroupsTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, request model.ImportRequest, entityIDs, eventIDs map[string]string, local map[string]resolvedNode, stats *model.ImportStats) error {
	if len(request.EntityGroupProposals) > 100 || len(request.EventGroupProposals) > 100 {
		return store.ErrInvalidValue
	}
	if request.Origin != "agent" && (len(request.EntityGroupProposals) > 0 || len(request.EventGroupProposals) > 0) {
		return store.ErrInvalidValue
	}
	var candidates []model.Group
	deviceGroups := map[string]int{}
	for _, entity := range request.Selection.Entities {
		method, _ := entity.Attributes["identity_method"].(string)
		if entity.TypeCode != "device" || method != "pt-nad-host-id" || !slices.ContainsFunc(entity.Provenance, func(p model.GatewayProvenance) bool { return p.Source == "pt-nad" }) {
			continue
		}
		g := newSourceGroup(scope, "entity", "resolved_entity", "device", "source-device:v1:"+entity.Value, "Device "+entity.Value)
		var evidence []string
		for _, event := range request.Selection.Events {
			if slices.ContainsFunc(event.Entities, func(e model.GatewayEventEntity) bool { return e.SnapshotID == entity.SnapshotID }) {
				evidence = append(evidence, eventIDs[event.SnapshotID])
			}
		}
		a := sourceAssertion(request.InvestigationID, entity.Value, "pt-nad-host-id", evidence)
		g.Members = append(g.Members, sourceMember(entityIDs[entity.SnapshotID], "subject", a))
		deviceGroups[entity.SnapshotID] = len(candidates)
		candidates = append(candidates, g)
	}
	for _, relation := range request.Selection.Relations {
		index, ok := deviceGroups[relation.SourceEntitySnapshotID]
		if !ok || relation.RelationCode != "has_identifier" || relation.Provenance.Source != "pt-nad" {
			continue
		}
		if relation.OccurredAt == nil || relation.OccurredAt.IsZero() {
			stats.Warnings = append(stats.Warnings, "Grouping skipped: source identifier has no observation time")
			continue
		}
		a := sourceAssertion(request.InvestigationID, relation.Provenance.Source+":"+relation.Provenance.ExternalID, "source-identifier", nil)
		a.ValidFrom, a.ValidTo = relation.OccurredAt, relation.OccurredAt
		m := sourceMember(entityIDs[relation.TargetEntitySnapshotID], "identifier", a)
		g := &candidates[index]
		prior := slices.IndexFunc(g.Members, func(x model.GroupMember) bool { return x.ObjectID == m.ObjectID })
		if prior < 0 {
			g.Members = append(g.Members, m)
		} else {
			g.Members[prior].Assertions = grouping.UnionAssertions(g.Members[prior].Assertions, m.Assertions)
		}
	}
	covered := map[string]bool{}
	for _, session := range request.Selection.Sessions {
		key := "source-session:v1:" + sourceGroupObjectKey(session.Ref)
		parentRef := session.Ref.SourceInstance + ":" + session.Ref.RecordType + ":" + session.Ref.ExternalID
		if session.Ref.SourceCode == "pt-nad" && session.Ref.SourceInstance != "" {
			key = sourceCompositeKey(session.Ref.SourceCode, parentRef)
		}
		g := newSourceGroup(scope, "event", "composite", "", key, session.Title)
		for _, snapshot := range session.EventSnapshotIDs {
			id, ok := eventIDs[snapshot]
			if !ok {
				return store.ErrUnknownReference
			}
			covered[snapshot] = true
			role := "part"
			for _, e := range request.Selection.Events {
				if e.SnapshotID == snapshot && e.Provenance.ExternalID == session.Ref.SourceInstance+":"+session.Ref.RecordType+":"+session.Ref.ExternalID {
					role = "parent"
				}
			}
			g.Members = append(g.Members, sourceMember(id, role, sourceAssertion(request.InvestigationID, sourceGroupObjectKey(session.Ref), "source-session", []string{id})))
		}
		if len(g.Members) > 0 {
			candidates = append(candidates, g)
		}
	}
	for _, finding := range request.Selection.Findings {
		g := newSourceGroup(scope, "event", "correlation", "", "source-finding:v1:"+sourceGroupObjectKey(finding.Ref), finding.Title)
		for _, snapshot := range finding.EventSnapshotIDs {
			id, ok := eventIDs[snapshot]
			if !ok {
				return store.ErrUnknownReference
			}
			g.Members = append(g.Members, sourceMember(id, "evidence", sourceAssertion(request.InvestigationID, sourceGroupObjectKey(finding.Ref), "source-finding", []string{id})))
		}
		if len(g.Members) > 0 {
			candidates = append(candidates, g)
		}
	}
	// Existing source rows are consulted only through this investigation's evidence membership.
	parents := map[string]int{}
	for _, child := range request.Selection.Events {
		parent, ok := sourceSubeventParent(child)
		if !ok || covered[child.SnapshotID] || parent == child.Provenance.ExternalID {
			continue
		}
		var parentID string
		err := tx.QueryRow(ctx, `SELECT e.id::text FROM events e JOIN investigation_events ie ON ie.event_id=e.id AND ie.project_id=e.project_id
 WHERE e.project_id=$1 AND e.source_code=$2 AND e.source_event_id=$3 AND ie.investigation_id=$4::uuid`, scope.ProjectID, child.Provenance.Source, parent, request.InvestigationID).Scan(&parentID)
		if errors.Is(err, pgx.ErrNoRows) {
			stats.Warnings = append(stats.Warnings, "Grouping skipped: source parent is not attached to this investigation")
			continue
		}
		if err != nil {
			return err
		}
		key := sourceCompositeKey(child.Provenance.Source, parent)
		index, exists := parents[key]
		if !exists {
			index = len(candidates)
			parents[key] = index
			g := newSourceGroup(scope, "event", "composite", "", key, "Source composite event")
			g.Members = append(g.Members, sourceMember(parentID, "parent", sourceAssertion(request.InvestigationID, key, "source-subevent", []string{parentID})))
			candidates = append(candidates, g)
		}
		id := eventIDs[child.SnapshotID]
		candidates[index].Members = append(candidates[index].Members, sourceMember(id, "part", sourceAssertion(request.InvestigationID, key, "source-subevent", []string{id})))
	}
	for _, candidate := range candidates {
		results, warnings, err := importSourceGroupTx(ctx, tx, candidate)
		if err != nil {
			return fmt.Errorf("import source %s group: %w", candidate.Family, err)
		}
		stats.Groups = append(stats.Groups, results...)
		stats.Warnings = append(stats.Warnings, warnings...)
	}
	for _, batch := range []struct {
		family    string
		proposals []model.GroupProposal
	}{{"entity", request.EntityGroupProposals}, {"event", request.EventGroupProposals}} {
		for _, proposal := range batch.proposals {
			result, err := importGroupProposalTx(ctx, tx, scope, request, batch.family, proposal, local)
			if err != nil {
				return err
			}
			stats.Groups = append(stats.Groups, result)
		}
	}
	// Multiple source assertions may contribute to the same group in one batch.
	byGroup := map[string]model.GroupImportResult{}
	for _, r := range stats.Groups {
		old, exists := byGroup[r.GroupID]
		if exists {
			r.MemberIDs = append(r.MemberIDs, old.MemberIDs...)
		}
		slices.Sort(r.MemberIDs)
		r.MemberIDs = slices.Compact(r.MemberIDs)
		byGroup[r.GroupID] = r
	}
	stats.Groups = []model.GroupImportResult{}
	for _, r := range byGroup {
		stats.Groups = append(stats.Groups, r)
	}
	slices.SortFunc(stats.Groups, func(a, b model.GroupImportResult) int { return strings.Compare(a.GroupID, b.GroupID) })
	return nil
}

func activeGroupLeavesTx(ctx context.Context, tx pgx.Tx, g model.Group, seen map[string]bool) ([]model.Group, error) {
	if seen[g.ID] {
		return nil, nil
	}
	seen[g.ID] = true
	if g.State == "active" {
		return []model.Group{g}, nil
	}
	var out []model.Group
	for _, id := range g.SuccessorIDs {
		next, err := getGroupTx(ctx, tx, g.GroupScope, g.Family, id)
		if err != nil {
			return nil, err
		}
		leaves, err := activeGroupLeavesTx(ctx, tx, next, seen)
		if err != nil {
			return nil, err
		}
		out = append(out, leaves...)
	}
	return out, nil
}

func importSourceGroupTx(ctx context.Context, tx pgx.Tx, candidate model.Group) ([]model.GroupImportResult, []string, error) {
	table, _, _, _, _, err := groupTables(candidate.Family)
	if err != nil {
		return nil, nil, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM `+table+` WHERE project_id=$1 AND root_investigation_id=$2::uuid AND group_key=$3`, candidate.ProjectID, candidate.RootID, candidate.Key).Scan(&id)
	isNew := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !isNew {
		return nil, nil, err
	}
	var targets []model.Group
	if isNew {
		targets = []model.Group{candidate}
		targets[0].Members = []model.GroupMember{}
	} else {
		g, e := getGroupTx(ctx, tx, candidate.GroupScope, candidate.Family, id)
		if e != nil {
			return nil, nil, e
		}
		targets, e = activeGroupLeavesTx(ctx, tx, g, map[string]bool{})
		if e != nil {
			return nil, nil, e
		}
	}
	var warnings []string
	results := []model.GroupImportResult{}
	for index := range targets {
		g := grouping.Clone(targets[index])
		before := []model.Group{grouping.Clone(g)}
		if isNew {
			before = nil
		}
		changed := isNew
		visible := map[string]bool{}
		for _, incoming := range candidate.Members {
			memberIndex := slices.IndexFunc(g.Members, func(m model.GroupMember) bool { return m.ObjectID == incoming.ObjectID })
			if len(targets) > 1 && memberIndex < 0 {
				continue
			}
			visible[incoming.ObjectID] = true
			if memberIndex < 0 {
				g.Members = append(g.Members, incoming)
				changed = true
				continue
			}
			m := &g.Members[memberIndex]
			merged := grouping.UnionAssertions(m.Assertions, incoming.Assertions)
			if len(merged) != len(m.Assertions) {
				m.Assertions = merged
				m.Version++
				changed = true
			}
			// Preserve reviewed status, role, order, confidence and reason on refresh.
		}
		if changed {
			if !isNew {
				g.Version++
			}
			if err = validateGroupEvidenceTx(ctx, tx, g); err != nil {
				return nil, nil, fmt.Errorf("validate attached evidence: %w", err)
			}
			if err = checkGroupUniquenessTx(ctx, tx, g); err != nil {
				return nil, nil, err
			}
			if err = saveGroupTx(ctx, tx, &g); err != nil {
				return nil, nil, fmt.Errorf("save group: %w", err)
			}
			hash, e := operationHash(g)
			if e != nil {
				return nil, nil, e
			}
			if err = recordGroupOperationTx(ctx, tx, g.GroupScope, "source:"+uuid.NewString(), hash, "source", "source", "Import explicit source relationships", before, []model.Group{g}); err != nil {
				return nil, nil, fmt.Errorf("record source operation: %w", err)
			}
		}
		result := groupResult(g)
		result.MemberIDs = []string{}
		for _, m := range g.Members {
			if visible[m.ObjectID] {
				result.MemberIDs = append(result.MemberIDs, m.ID)
			}
		}
		if len(result.MemberIDs) > 0 {
			results = append(results, result)
		}
	}
	if len(targets) != 1 {
		for _, m := range candidate.Members {
			known := false
			for _, g := range targets {
				known = known || slices.ContainsFunc(g.Members, func(x model.GroupMember) bool { return x.ObjectID == m.ObjectID })
			}
			if !known {
				warnings = append(warnings, "Grouping skipped: a new source member has no unambiguous successor after split")
			}
		}
	}
	return results, warnings, nil
}

func importGroupProposalTx(ctx context.Context, tx pgx.Tx, scope model.GroupScope, request model.ImportRequest, family string, p model.GroupProposal, local map[string]resolvedNode) (model.GroupImportResult, error) {
	if request.Origin != "agent" || !grouping.ValidID(p.ProposalID) || strings.TrimSpace(p.Why) == "" || len(p.Why) > 4096 || len(p.Members) == 0 || len(p.Members) > 2500 || len(p.EvidenceEventRefs) == 0 || len(p.EvidenceEventRefs) > 500 {
		return model.GroupImportResult{}, store.ErrInvalidValue
	}
	evidence := []string{}
	for _, ref := range p.EvidenceEventRefs {
		n, ok := local[ref]
		if !ok || n.EventID == nil {
			return model.GroupImportResult{}, store.ErrUnknownReference
		}
		evidence = append(evidence, *n.EventID)
	}
	slices.Sort(evidence)
	evidence = slices.Compact(evidence)
	g := newSourceGroup(scope, family, p.Kind, p.TypeCode, "agent:"+p.ProposalID, p.Title)
	// Human/agent input must be validated, not silently truncated like a source label.
	g.Title = p.Title
	objects := []string{}
	for _, spec := range p.Members {
		n, ok := local[spec.NodeRef]
		if !ok || n.NodeType != family {
			return model.GroupImportResult{}, store.ErrUnknownReference
		}
		var id string
		if family == "entity" && n.Node.EntityID != nil {
			id = *n.Node.EntityID
		} else if family == "event" && n.EventID != nil {
			id = *n.EventID
		} else {
			return model.GroupImportResult{}, store.ErrUnknownReference
		}
		objects = append(objects, id)
		a := model.GroupAssertion{InvestigationID: request.InvestigationID, Origin: "agent", OriginRef: strings.Join(request.SomIssueIDs, ","), Method: "agent-proposal", MethodVersion: "1", EvidenceEventIDs: evidence, ValidFrom: spec.ValidFrom, ValidTo: spec.ValidTo, Reason: p.Why}
		g.Members = append(g.Members, model.GroupMember{ID: uuid.NewString(), ObjectID: id, Role: spec.Role, Ordinal: spec.Ordinal, Status: "proposed", Confidence: spec.Confidence, Reason: p.Why, Version: 1, Assertions: []model.GroupAssertion{a}})
	}
	if err := grouping.Validate(g); err != nil {
		return model.GroupImportResult{}, groupDomainError(err)
	}
	hash, err := operationHash(struct {
		Investigation, Family     string
		Proposal                  model.GroupProposal
		Objects, Evidence, Issues []string
	}{request.InvestigationID, family, p, objects, evidence, request.SomIssueIDs})
	if err != nil {
		return model.GroupImportResult{}, err
	}
	if prior, found, e := operationRetryTx(ctx, tx, scope, "proposal:"+p.ProposalID, hash); found || e != nil {
		if e != nil {
			return model.GroupImportResult{}, e
		}
		if len(prior) != 1 {
			return model.GroupImportResult{}, store.ErrInvalidValue
		}
		return groupResult(prior[0]), nil
	}
	if err = validateGroupEvidenceTx(ctx, tx, g); err != nil {
		return model.GroupImportResult{}, err
	}
	if err = saveGroupTx(ctx, tx, &g); err != nil {
		return model.GroupImportResult{}, err
	}
	if err = recordGroupOperationTx(ctx, tx, scope, "proposal:"+p.ProposalID, hash, "proposal", "agent:"+request.SomIssueIDs[0], p.Why, nil, []model.Group{g}); err != nil {
		return model.GroupImportResult{}, err
	}
	return groupResult(g), nil
}
