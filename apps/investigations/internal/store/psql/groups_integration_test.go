package psql

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"slices"
	"sync"
	"testing"
	"time"
)

func groupCase(t *testing.T, db *DB, project string, parent *string) string {
	t.Helper()
	v, err := db.CreateInvestigation(context.Background(), model.InvestigationNew{ProjectID: project, ParentID: parent, Title: "group test"})
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}
func groupSelection(device string) model.GatewaySelection {
	at := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	provenance := func(id string) model.GatewayProvenance {
		return model.GatewayProvenance{Source: "pt-nad", ExternalID: id, FetchedAt: at}
	}
	return model.GatewaySelection{
		Entities:  []model.GatewayEntity{{SnapshotID: "device", TypeCode: "device", Value: "pt-nad:" + device, Attributes: map[string]any{"identity_method": "pt-nad-host-id", "source_instance": "19"}, Provenance: []model.GatewayProvenance{provenance("device:" + device)}}, {SnapshotID: "ip", TypeCode: "ip", Value: "10.0.0.1", Provenance: []model.GatewayProvenance{provenance("ip:10.0.0.1")}}},
		Events:    []model.GatewayEvent{{SnapshotID: "parent", Direct: true, Title: "session", EventType: "network_session", OccurredAt: at, Provenance: provenance("parent:" + device), Entities: []model.GatewayEventEntity{{SnapshotID: "device", Roles: []string{"mentions"}}, {SnapshotID: "ip", Roles: []string{"mentions"}}}}, {SnapshotID: "part", Direct: true, Title: "child", EventType: "file", OccurredAt: at, Provenance: provenance("part:" + device), Attributes: map[string]any{"relation_type": "subevent_of", "parent_source_event_id": "parent:" + device}}},
		Relations: []model.GatewayRelation{{SnapshotID: "device-ip", RelationCode: "has_identifier", SourceEntitySnapshotID: "device", TargetEntitySnapshotID: "ip", OccurredAt: &at, Provenance: provenance("identifier:" + device)}},
	}
}
func importGroupFixture(t *testing.T, db *DB, project, inv, device string) model.ImportStats {
	t.Helper()
	v, err := db.ImportContext(context.Background(), model.ImportRequest{ProjectID: project, InvestigationID: inv, Origin: "analyst", Selection: groupSelection(device)})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Groups) != 2 {
		t.Fatalf("expected entity and composite groups: %+v", v)
	}
	return v
}
func importedGroup(t *testing.T, db *DB, scope model.GroupScope, stats model.ImportStats, family string) model.Group {
	t.Helper()
	for _, g := range stats.Groups {
		if g.Family == family {
			v, err := db.GetGroup(context.Background(), scope, family, g.GroupID)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
	}
	t.Fatal("missing group")
	return model.Group{}
}
func groupCleanup(t *testing.T, db *DB, project string) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range []string{"investigations", "events", "entities"} {
			if _, err := db.Pgx().Exec(context.Background(), `DELETE FROM `+table+` WHERE project_id=$1`, project); err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	})
}

func TestGroupingTreeIsolationAndProjection(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	a := groupCase(t, db, project, nil)
	a1 := groupCase(t, db, project, &a)
	a11 := groupCase(t, db, project, &a1)
	a2 := groupCase(t, db, project, &a)
	b := groupCase(t, db, project, nil)
	scopeA := model.GroupScope{ProjectID: project, RootID: a}
	scopeB := model.GroupScope{ProjectID: project, RootID: b}
	first := importGroupFixture(t, db, project, a11, "one")
	sibling := importGroupFixture(t, db, project, a2, "one")
	separate := importGroupFixture(t, db, project, b, "one")
	ga := importedGroup(t, db, scopeA, first, "entity")
	gs := importedGroup(t, db, scopeA, sibling, "entity")
	gb := importedGroup(t, db, scopeB, separate, "entity")
	if ga.ID != gs.ID || ga.ID == gb.ID || ga.Members[0].ObjectID != gb.Members[0].ObjectID && ga.Members[0].ObjectID != gb.Members[1].ObjectID {
		t.Fatal("tree group namespace or shared atoms incorrect")
	}
	for _, scope := range []model.GroupScope{scopeA, {ProjectID: "ffffffffffff", RootID: b}, {ProjectID: project, RootID: a11}} {
		if _, err := db.GetGroup(ctx, scope, "entity", gb.ID); !errors.Is(err, store.ErrRecordNotFound) {
			t.Fatalf("foreign group detail: %v", err)
		}
		if _, err := db.GroupHistory(ctx, scope, "entity", gb.ID, nil, 50); !errors.Is(err, store.ErrRecordNotFound) {
			t.Fatalf("foreign history: %v", err)
		}
	}
	r := model.ProjectionRequest{ProjectID: project, InvestigationID: a1}
	out, err := db.GraphProjection(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RawNodes) != 0 {
		t.Fatal("implicit subtree leak")
	}
	r.Filter.IncludeSubtree = true
	out, err = db.GraphProjection(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RawNodes) != 4 || out.RootID != a {
		t.Fatalf("subtree projection: %+v", out)
	}
	for _, n := range out.RawNodes {
		if n.InvestigationID != a11 {
			t.Fatal("sibling node leaked")
		}
	}
	// Add evidence to the sibling only. Its identifiers and aggregates stay outside a11.
	selection := groupSelection("one")
	extra := selection.Events[1]
	extra.SnapshotID = "extra"
	extra.Provenance.ExternalID = "extra"
	extra.Title = "sibling secret"
	selection.Events = append(selection.Events, extra)
	if _, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: a2, Origin: "analyst", Selection: selection}); err != nil {
		t.Fatal(err)
	}
	r.InvestigationID = a11
	r.Filter.IncludeSubtree = false
	out, err = db.GraphProjection(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RawNodes) != 4 || len(out.Nodes) != 2 {
		t.Fatalf("projection size or collapse: %+v", out)
	}
	for _, n := range out.RawNodes {
		if n.Label != nil && *n.Label == "sibling secret" {
			t.Fatal("sibling metadata leak")
		}
	}
	visibleNodes := map[string]bool{}
	for _, n := range out.RawNodes {
		visibleNodes[n.ID] = true
	}
	for _, g := range out.Groups {
		for _, m := range g.Members {
			for _, id := range m.NodeIDs {
				if !visibleNodes[id] {
					t.Fatal("hidden group member disclosed")
				}
			}
			for _, a := range m.Assertions {
				if a.InvestigationID != a11 {
					t.Fatal("sibling assertion disclosed")
				}
			}
		}
	}
	hyp, err := db.CreateHypothesis(ctx, model.HypothesisNew{ProjectID: project, InvestigationID: a11, Statement: "one event"})
	if err != nil {
		t.Fatal(err)
	}
	var eventNode string
	for _, n := range out.RawNodes {
		if n.NodeType == "event" {
			eventNode = n.ID
			break
		}
	}
	if err := db.AddHypothesisNode(ctx, project, a11, hyp.ID, eventNode); err != nil {
		t.Fatal(err)
	}
	r.HypothesisID = &hyp.ID
	out, err = db.GraphProjection(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.RawNodes) != 1 || len(out.RawEdges) != 0 {
		t.Fatalf("hypothesis expanded outside membership: %+v", out)
	}
	if err := db.DeleteInvestigation(ctx, project, a2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GraphProjection(ctx, model.ProjectionRequest{ProjectID: project, InvestigationID: a2}); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("deleted view: %v", err)
	}
	if _, err := db.GetGroup(ctx, scopeA, "entity", ga.ID); err != nil {
		t.Fatal("child deletion hid tree history", err)
	}
	if err := db.DeleteInvestigation(ctx, project, a); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetGroup(ctx, scopeA, "entity", ga.ID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("deleted root group: %v", err)
	}
	if _, err := db.GetGroup(ctx, scopeB, "entity", gb.ID); err != nil {
		t.Fatal("root A deletion affected B", err)
	}
}

func TestGroupingReviewIdempotencySourceRefreshAndAudit(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	root := groupCase(t, db, project, nil)
	scope := model.GroupScope{ProjectID: project, RootID: root}
	stats := importGroupFixture(t, db, project, root, "one")
	g := importedGroup(t, db, scope, stats, "event")
	part := g.Members[slices.IndexFunc(g.Members, func(m model.GroupMember) bool { return m.Role == "part" })]
	r := model.GroupMutation{GroupScope: scope, Family: "event", GroupID: g.ID, Actor: "analyst:test", Review: &model.GroupReview{OperationID: uuid.NewString(), Version: g.Version, Reason: "not part of the session", Members: []model.GroupReviewMember{{ID: part.ID, Version: part.Version, Status: "rejected"}}}}
	changed, err := db.MutateGroup(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.MutateGroup(ctx, r)
	if err != nil || again[0].Version != changed[0].Version {
		t.Fatal("identical retry not idempotent", err)
	}
	r.Review.Reason = "changed payload"
	if _, err := db.MutateGroup(ctx, r); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("reused operation accepted: %v", err)
	}
	r.Review.OperationID = uuid.NewString()
	if _, err := db.MutateGroup(ctx, r); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	refreshed := importGroupFixture(t, db, project, root, "one")
	g = importedGroup(t, db, scope, refreshed, "event")
	for _, m := range g.Members {
		if m.ID == part.ID && m.Status != "rejected" {
			t.Fatal("refresh revived rejected member")
		}
	}
	history, err := db.GroupHistory(ctx, scope, "event", g.ID, nil, 1)
	if err != nil || len(history.Operations) != 1 || history.NextCursor == nil {
		t.Fatal("audit pagination", err)
	}
	next, err := db.GroupHistory(ctx, scope, "event", g.ID, history.NextCursor, 1)
	if err != nil || len(next.Operations) != 1 || next.Operations[0].ID == history.Operations[0].ID {
		t.Fatal("audit next page", err)
	}
	if _, err := db.Pgx().Exec(ctx, `UPDATE group_operations SET reason='tampered' WHERE id=$1::uuid`, history.Operations[0].ID); err == nil {
		t.Fatal("audit update permitted")
	}
	if _, err := db.Pgx().Exec(ctx, `DELETE FROM group_operations WHERE id=$1::uuid`, history.Operations[0].ID); err == nil {
		t.Fatal("audit deletion permitted")
	}
}

func TestGroupingMergeSplitRefreshAndConcurrentReview(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	root := groupCase(t, db, project, nil)
	scope := model.GroupScope{ProjectID: project, RootID: root}
	first := importGroupFixture(t, db, project, root, "one")
	second := importGroupFixture(t, db, project, root, "two")
	a := importedGroup(t, db, scope, first, "event")
	b := importedGroup(t, db, scope, second, "event")
	merge := model.GroupMerge{OperationID: uuid.NewString(), Version: a.Version, Reason: "one composite", Sources: []model.GroupVersion{{ID: b.ID, Version: b.Version}}}
	for _, m := range a.Members {
		merge.Members = append(merge.Members, model.GroupPlacement{MemberID: m.ID, Role: m.Role})
	}
	for _, m := range b.Members {
		merge.Members = append(merge.Members, model.GroupPlacement{MemberID: m.ID, Role: "part"})
	}
	updated, err := db.MutateGroup(ctx, model.GroupMutation{GroupScope: scope, Family: "event", GroupID: a.ID, Actor: "test", Merge: &merge})
	if err != nil {
		t.Fatal(err)
	}
	a = updated[0]
	refreshed := importGroupFixture(t, db, project, root, "two")
	merged := importedGroup(t, db, scope, refreshed, "event")
	if merged.ID != a.ID || len(merged.Members) != 4 {
		t.Fatal("refresh resurrected merge source")
	}
	split := model.GroupSplit{OperationID: uuid.NewString(), Version: merged.Version, Reason: "separate occurrences", Partitions: []model.GroupPartition{{Title: "one"}, {Title: "two"}}}
	for _, m := range merged.Members {
		which := 0
		role := m.Role
		for _, original := range b.Members {
			if original.ObjectID == m.ObjectID {
				which = 1
				role = original.Role
			}
		}
		split.Partitions[which].Members = append(split.Partitions[which].Members, model.GroupPlacement{MemberID: m.ID, Role: role})
	}
	parts, err := db.MutateGroup(ctx, model.GroupMutation{GroupScope: scope, Family: "event", GroupID: a.ID, Actor: "test", Split: &split})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || parts[0].State != "superseded" {
		t.Fatal("split lineage incorrect")
	}
	selection := groupSelection("two")
	extra := selection.Events[1]
	extra.SnapshotID = "extra"
	extra.Provenance.ExternalID = "new after split"
	selection.Events = append(selection.Events, extra)
	refreshed, err = db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: root, Origin: "analyst", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.Warnings) == 0 {
		t.Fatal("ambiguous new split member not warned")
	}
	current := importedGroup(t, db, scope, refreshed, "event")
	if current.ID != parts[2].ID || len(current.Members) != 2 {
		t.Fatalf("refresh did not route known members: %+v", current)
	}
	// Optimistic concurrency is enforced even when both requests observed one version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := current.Members[0]
			_, e := db.MutateGroup(ctx, model.GroupMutation{GroupScope: scope, Family: "event", GroupID: current.ID, Actor: "test", Review: &model.GroupReview{OperationID: uuid.NewString(), Version: current.Version, Reason: "concurrent confirmation", Members: []model.GroupReviewMember{{ID: m.ID, Version: m.Version, Status: "confirmed"}}}})
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for e := range results {
		if e == nil {
			successes++
		} else if errors.Is(e, store.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrency: %d success %d conflict", successes, conflicts)
	}
}

func TestGroupingAgentProposalsAtomicAndScoped(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	root := groupCase(t, db, project, nil)
	other := groupCase(t, db, project, nil)
	scope := model.GroupScope{ProjectID: project, RootID: root}
	importGroupFixture(t, db, project, root, "one")
	importGroupFixture(t, db, project, other, "one")
	raw, err := db.GraphNodes(ctx, project, root, model.NodeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	request := model.ImportRequest{ProjectID: project, InvestigationID: root, Origin: "agent", SomIssueIDs: []string{uuid.NewString()}}
	proposal := model.GroupProposal{ProposalID: uuid.NewString(), Kind: "same_event", Title: "one occurrence", Why: "same source record", EvidenceEventRefs: []string{"ev0"}}
	for _, n := range raw {
		if n.NodeType != "event" {
			continue
		}
		ref := fmt.Sprintf("ev%d", len(request.Nodes))
		id := n.ID
		request.Nodes = append(request.Nodes, model.AgentNode{Ref: ref, NodeID: &id})
		role := "duplicate"
		if len(proposal.Members) == 0 {
			role = "primary"
		}
		proposal.Members = append(proposal.Members, model.GroupProposalMember{NodeRef: ref, Role: role})
	}
	request.EventGroupProposals = []model.GroupProposal{proposal}
	stats, err := db.ImportContext(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	g := importedGroup(t, db, scope, stats, "event")
	for _, m := range g.Members {
		if m.Status != "proposed" || m.Assertions[0].Origin != "agent" {
			t.Fatal("agent confirmed membership")
		}
	}
	again, err := db.ImportContext(ctx, request)
	if err != nil || again.Groups[0].GroupID != g.ID {
		t.Fatal("proposal retry", err)
	}
	request.EventGroupProposals[0].Why = "changed"
	if _, err := db.ImportContext(ctx, request); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("changed proposal reused ID: %v", err)
	}
	request.EventGroupProposals[0].ProposalID = uuid.NewString()
	request.EventGroupProposals[0].Members[0].NodeRef = "unknown"
	fresh := groupSelection("rollback")
	request.Selection = fresh
	if _, err := db.ImportContext(ctx, request); !errors.Is(err, store.ErrUnknownReference) {
		t.Fatalf("invalid local reference: %v", err)
	}
	var count int
	if err := db.Pgx().QueryRow(ctx, `SELECT count(*) FROM entities WHERE project_id=$1 AND canonical_key='pt-nad:rollback'`, project).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed proposal did not roll back atomic import", err)
	}
	request.Selection = model.GatewaySelection{}
	request.InvestigationID = other
	request.EventGroupProposals = nil
	if _, err := db.ImportContext(ctx, request); !errors.Is(err, store.ErrUnknownReference) {
		t.Fatalf("foreign node ref accepted: %v", err)
	}
}

func TestGroupingHistoryIndexesExist(t *testing.T) {
	db := softDeleteTestDB(t)
	expected := []string{
		"ix_group_operation_links_entity_history",
		"ix_group_operation_links_event_history",
	}
	rows, err := db.Pgx().Query(context.Background(), `SELECT indexname FROM pg_indexes
		WHERE schemaname=current_schema() AND indexname=ANY($1::text[])`, expected)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range expected {
		if !seen[name] {
			t.Fatalf("history index %s is missing", name)
		}
	}
}

func TestGroupingSourceObjectImportsReuseRawComposite(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	root := groupCase(t, db, project, nil)
	scope := model.GroupScope{ProjectID: project, RootID: root}
	selection := groupSelection("objects")
	at := selection.Events[0].OccurredAt
	parentRef := "19:nad_session:object-1"
	selection.Events[0].Provenance.ExternalID = parentRef
	selection.Events[1].Attributes["parent_source_event_id"] = parentRef
	first, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: root, Origin: "analyst", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	original := importedGroup(t, db, scope, first, "event")
	ref := model.GatewayObjectRef{SourceCode: "pt-nad", SourceInstance: "19", RecordType: "nad_session", ExternalID: "object-1", TimeRange: model.GatewayTimeRange{From: at.Add(-time.Hour), To: at.Add(time.Hour)}}
	selection.Sessions = []model.GatewaySession{{SnapshotID: "session", Ref: ref, Title: "source session", Severity: "low", StartedAt: at, FetchedAt: at, ContextStatus: "complete", Direct: true, Normalized: []byte(`{}`), Provenance: []byte(`{}`), EventSnapshotIDs: []string{"parent", "part"}, EntitySnapshotIDs: []string{"device", "ip"}}}
	ref.RecordType = "nad_attack"
	ref.ExternalID = "attack-1"
	selection.Findings = []model.GatewayFinding{{SnapshotID: "finding", Ref: ref, Kind: "nad_attack", Title: "source finding", Severity: "low", OccurredAt: at, FetchedAt: at, ContextStatus: "complete", Direct: true, Normalized: []byte(`{}`), Provenance: []byte(`{}`), EventSnapshotIDs: []string{"parent", "part"}}}
	stats, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: root, Origin: "analyst", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Groups) != 3 {
		t.Fatalf("session/finding groups: %+v", stats)
	}
	found := false
	for _, item := range stats.Groups {
		if item.GroupID == original.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("source object import duplicated raw composite")
	}
	// Repeating source resolution does not create decisions or bump versions.
	group, err := db.GetGroup(ctx, scope, "event", original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: root, Origin: "analyst", Selection: selection}); err != nil {
		t.Fatal(err)
	}
	after, err := db.GetGroup(ctx, scope, "event", original.ID)
	if err != nil || after.Version != group.Version {
		t.Fatal("source resolution not idempotent", err)
	}
}

func TestGroupingIdentityConflictsAreTreeLocal(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	root := groupCase(t, db, project, nil)
	scope := model.GroupScope{ProjectID: project, RootID: root}
	a := importedGroup(t, db, scope, importGroupFixture(t, db, project, root, "one"), "entity")
	b := importedGroup(t, db, scope, importGroupFixture(t, db, project, root, "two"), "entity")
	merge := model.GroupMerge{OperationID: uuid.NewString(), Version: a.Version, Reason: "claimed same device", Sources: []model.GroupVersion{{ID: b.ID, Version: b.Version}}}
	for _, g := range []model.Group{a, b} {
		for _, m := range g.Members {
			merge.Members = append(merge.Members, model.GroupPlacement{MemberID: m.ID, Role: m.Role})
		}
	}
	if _, err := db.MutateGroup(ctx, model.GroupMutation{GroupScope: scope, Family: "entity", GroupID: a.ID, Actor: "test", Merge: &merge}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("contradictory HostIDs merged: %v", err)
	}
	unchanged, err := db.GetGroup(ctx, scope, "entity", b.ID)
	if err != nil || unchanged.State != "active" {
		t.Fatal("failed merge was not atomic", err)
	}
	// Same event memberships may be confirmed independently in separate roots,
	// but a second confirmed identity inside one root conflicts.
	other := groupCase(t, db, project, nil)
	importGroupFixture(t, db, project, other, "one")
	for index, inv := range []string{root, other, root} {
		nodes, err := db.GraphNodes(ctx, project, inv, model.NodeFilter{})
		if err != nil {
			t.Fatal(err)
		}
		request := model.ImportRequest{ProjectID: project, InvestigationID: inv, Origin: "agent", SomIssueIDs: []string{uuid.NewString()}}
		p := model.GroupProposal{ProposalID: uuid.NewString(), Kind: "same_event", Title: "same occurrence", Why: "explicit evidence", EvidenceEventRefs: []string{"event"}}
		for _, n := range nodes {
			if n.EventID != nil {
				id := n.ID
				request.Nodes = []model.AgentNode{{Ref: "event", NodeID: &id}}
				p.Members = []model.GroupProposalMember{{NodeRef: "event", Role: "primary"}}
				break
			}
		}
		// Choose the same canonical event in both scopes, independent of node ordering.
		var eventID string
		if err := db.Pgx().QueryRow(ctx, `SELECT id::text FROM events WHERE project_id=$1 AND source_code='pt-nad' AND source_event_id='parent:one'`, project).Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		request.Nodes = []model.AgentNode{{Ref: "event", EventID: &eventID}}
		p.Members = []model.GroupProposalMember{{NodeRef: "event", Role: "primary"}}
		request.EventGroupProposals = []model.GroupProposal{p}
		stats, err := db.ImportContext(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		gs := model.GroupScope{ProjectID: project, RootID: inv}
		g := importedGroup(t, db, gs, stats, "event")
		m := g.Members[0]
		_, err = db.MutateGroup(ctx, model.GroupMutation{GroupScope: gs, Family: "event", GroupID: g.ID, Actor: "test", Review: &model.GroupReview{OperationID: uuid.NewString(), Version: g.Version, Reason: "confirmed", Members: []model.GroupReviewMember{{ID: m.ID, Version: m.Version, Status: "confirmed"}}}})
		if index < 2 && err != nil {
			t.Fatal("independent root identity conflicted", err)
		}
		if index == 2 && !errors.Is(err, store.ErrConflict) {
			t.Fatalf("same-tree double identity: %v", err)
		}
	}
}
