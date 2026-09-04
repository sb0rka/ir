package grouping

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"slices"
	"testing"
	"time"
)

func testGroup(kind string, roles ...string) model.Group {
	g := model.Group{ID: uuid.NewString(), GroupScope: model.GroupScope{ProjectID: "test", RootID: uuid.NewString()}, Family: "event", Kind: kind, Title: "test", State: "active", Version: 1}
	if kind == "resolved_entity" {
		g.Family = "entity"
		g.TypeCode = "device"
	}
	for i, role := range roles {
		m := model.GroupMember{ID: uuid.NewString(), ObjectID: uuid.NewString(), Role: role, Status: "confirmed", Version: 1, Assertions: []model.GroupAssertion{{InvestigationID: g.RootID, Origin: "source", Method: "test", MethodVersion: "1", EvidenceEventIDs: []string{}}}}
		if kind == "sequence" {
			n := i
			m.Ordinal = &n
		}
		g.Members = append(g.Members, m)
	}
	return g
}

func TestGroupValidationAndReview(t *testing.T) {
	for _, g := range []model.Group{testGroup("resolved_entity", "subject", "identifier"), testGroup("same_event", "primary", "duplicate"), testGroup("composite", "parent", "part"), testGroup("sequence", "step", "step"), testGroup("correlation", "evidence")} {
		if err := Validate(g); err != nil {
			t.Fatalf("%s: %v", g.Kind, err)
		}
	}
	g := testGroup("same_event", "primary", "duplicate")
	r := model.GroupReview{OperationID: uuid.NewString(), Version: 1, Reason: "not the same occurrence", Members: []model.GroupReviewMember{{ID: g.Members[0].ID, Version: 1, Status: "rejected"}}}
	if _, err := Review(g, r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reject primary only: %v", err)
	}
	r.Members = append(r.Members, model.GroupReviewMember{ID: g.Members[1].ID, Version: 1, Status: "rejected"})
	changed, err := Review(g, r)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version != 2 || g.Members[0].Status != "confirmed" {
		t.Fatal("review mutated input or lost version")
	}
	if _, err := Review(changed, r); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale review: %v", err)
	}
	g = testGroup("sequence", "step", "step")
	g.Members[1].Ordinal = g.Members[0].Ordinal
	if err := Validate(g); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate sequence order accepted")
	}
	g = testGroup("resolved_entity", "identifier")
	if err := Validate(g); !errors.Is(err, ErrInvalid) {
		t.Fatal("identifier-only entity accepted")
	}
}

func TestMergePreservesTargetMemberAndScope(t *testing.T) {
	g := testGroup("correlation", "evidence")
	s := testGroup("correlation", "evidence")
	s.GroupScope = g.GroupScope
	s.Members[0].ObjectID = g.Members[0].ObjectID
	r := model.GroupMerge{OperationID: uuid.NewString(), Version: 1, Reason: "same context", Members: []model.GroupPlacement{{MemberID: s.Members[0].ID, Role: "evidence"}, {MemberID: g.Members[0].ID, Role: "evidence"}}}
	out, err := Merge(g, []model.Group{s}, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Members) != 1 || out.Members[0].ID != g.Members[0].ID || out.Members[0].Version != 2 {
		t.Fatalf("source-first merge lost target identity: %+v", out)
	}
	s.RootID = uuid.NewString()
	if _, err = Merge(g, []model.Group{s}, r); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tree merge accepted: %v", err)
	}
}

func TestSplitCoverageAndIdentifierSharing(t *testing.T) {
	g := testGroup("resolved_entity", "subject", "subject", "identifier")
	r := model.GroupSplit{OperationID: uuid.NewString(), Version: 1, Reason: "two devices", Partitions: []model.GroupPartition{{Title: "first", Members: []model.GroupPlacement{{MemberID: g.Members[0].ID, Role: "subject"}, {MemberID: g.Members[2].ID, Role: "identifier"}}}, {Title: "second", Members: []model.GroupPlacement{{MemberID: g.Members[1].ID, Role: "subject"}, {MemberID: g.Members[2].ID, Role: "identifier"}}}}}
	out, err := Split(g, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].ID == out[1].ID || out[0].RootID != g.RootID {
		t.Fatal("invalid split identities")
	}
	r.Partitions[1].Members[0].MemberID = g.Members[0].ID
	if _, err := Split(g, r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicated subject: %v", err)
	}
}

func projectionFixture(g model.Group) ([]model.GraphNode, []model.GraphEdge) {
	nodes := []model.GraphNode{}
	for _, m := range g.Members {
		id := m.ObjectID
		n := model.GraphNode{ID: uuid.NewString(), InvestigationID: g.RootID, NodeType: g.Family}
		if g.Family == "entity" {
			n.EntityID = &id
		} else {
			n.EventID = &id
		}
		nodes = append(nodes, n)
	}
	return nodes, []model.GraphEdge{}
}

func TestProjectionScopeStatusesAndLosslessAggregation(t *testing.T) {
	g := testGroup("same_event", "primary", "duplicate")
	nodes, _ := projectionFixture(g)
	outside := testGroup("same_event", "primary", "duplicate")
	outside.Members = g.Members
	other := model.GraphNode{ID: uuid.NewString(), InvestigationID: g.RootID, NodeType: "entity"}
	nodes = append(nodes, other)
	conf := float32(.8)
	edges := []model.GraphEdge{{ID: uuid.NewString(), InvestigationID: g.RootID, SourceNodeID: nodes[0].ID, TargetNodeID: nodes[1].ID, RelationCode: "subevent_of", Status: "confirmed", Origin: "source"},
		{ID: uuid.NewString(), InvestigationID: g.RootID, SourceNodeID: nodes[0].ID, TargetNodeID: other.ID, RelationCode: "mentions", Status: "confirmed", Origin: "source", Confidence: &conf, EvidenceEventIDs: []string{g.Members[0].ObjectID, uuid.NewString()}},
		{ID: uuid.NewString(), InvestigationID: g.RootID, SourceNodeID: nodes[1].ID, TargetNodeID: other.ID, RelationCode: "mentions", Status: "confirmed", Origin: "analyst"},
		{ID: uuid.NewString(), InvestigationID: g.RootID, SourceNodeID: nodes[1].ID, TargetNodeID: other.ID, RelationCode: "mentions", Status: "proposed", Origin: "agent"}}
	r := model.ProjectionRequest{InvestigationID: g.RootID}
	out := Project(r, g.GroupScope, nodes, edges, []model.Group{g, outside})
	if len(out.Nodes) != 2 || len(out.Edges) != 2 || len(out.RawNodes) != 3 || len(out.RawEdges) != 4 {
		t.Fatalf("not lossless: %+v", out)
	}
	for _, e := range out.Edges {
		if e.Status == "confirmed" && (len(e.MemberEdgeIDs) != 2 || len(e.Origins) != 2 || len(e.EvidenceEventIDs) != 1) {
			t.Fatalf("bad aggregate: %+v", e)
		}
	}
	a, _ := json.Marshal(out)
	slices.Reverse(nodes)
	slices.Reverse(edges)
	b, _ := json.Marshal(Project(r, g.GroupScope, nodes, edges, []model.Group{outside, g}))
	if string(a) != string(b) {
		t.Fatal("projection depends on input order")
	}
	g.Members[1].Status = "proposed"
	out = Project(r, g.GroupScope, nodes, edges, []model.Group{g})
	if len(out.Nodes) != 3 {
		t.Fatal("proposed node hidden")
	}
	if len(out.Groups) != 1 || len(out.Groups[0].Members) != 2 || !slices.ContainsFunc(out.Groups[0].Members, func(m model.ProjectionGroupMember) bool { return m.Status == "proposed" }) {
		t.Fatal("proposal not discoverable in view")
	}
	// A hidden evidence reference prevents its assertion from affecting this view.
	g.Members[0].Assertions[0].EvidenceEventIDs = []string{uuid.NewString()}
	out = Project(r, g.GroupScope, nodes, edges, []model.Group{g})
	if slices.ContainsFunc(out.Nodes, func(n model.ProjectionNode) bool { return n.GroupID != nil }) {
		t.Fatal("hidden evidence changed projection")
	}
}

func TestProjectionTemporalAmbiguityAndGaps(t *testing.T) {
	g := testGroup("resolved_entity", "subject", "identifier")
	nodes, _ := projectionFixture(g)
	at := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	ev := uuid.NewString()
	event := model.GraphNode{ID: uuid.NewString(), InvestigationID: g.RootID, NodeType: "event", EventID: &ev, OccurredAt: &at}
	nodes = append(nodes, event)
	edges := []model.GraphEdge{{ID: uuid.NewString(), InvestigationID: g.RootID, SourceNodeID: event.ID, TargetNodeID: nodes[1].ID, Status: "confirmed", RelationCode: "mentions"}}
	before, after := at.Add(-time.Hour), at.Add(time.Hour)
	a := g.Members[1].Assertions[0]
	a.ValidFrom = &before
	a.ValidTo = &before
	b := a
	b.ValidFrom = &after
	b.ValidTo = &after
	g.Members[1].Assertions = []model.GroupAssertion{a, b}
	out := Project(model.ProjectionRequest{InvestigationID: g.RootID}, g.GroupScope, nodes, edges, []model.Group{g})
	if len(out.Nodes) != 3 || len(out.Diagnostics) != 1 {
		t.Fatal("interval gap collapsed")
	}
	g.Members[1].Assertions[0].ValidTo = &after
	out = Project(model.ProjectionRequest{InvestigationID: g.RootID}, g.GroupScope, nodes, edges, []model.Group{g})
	if len(out.Nodes) != 2 {
		t.Fatalf("eligible identifier not collapsed: %+v", out)
	}
	second := testGroup("resolved_entity", "subject", "identifier")
	second.GroupScope = g.GroupScope
	second.Members[0].Assertions[0].InvestigationID = g.RootID
	second.Members[1] = g.Members[1]
	newNodes, _ := projectionFixture(second)
	nodes = append(nodes, newNodes[0])
	out = Project(model.ProjectionRequest{InvestigationID: g.RootID}, g.GroupScope, nodes, edges, []model.Group{g, second})
	if len(out.Nodes) != 4 || len(out.Diagnostics) != 1 {
		t.Fatal("shared IP picked arbitrary owner")
	}
}

func TestProjectionCompositeOverlapAndAnnotations(t *testing.T) {
	g := testGroup("composite", "parent", "part")
	nodes, edges := projectionFixture(g)
	h := Clone(g)
	h.ID = uuid.NewString()
	seq := Clone(g)
	seq.ID = uuid.NewString()
	seq.Kind = "sequence"
	for i := range seq.Members {
		v := 1 - i
		seq.Members[i].Role = "step"
		seq.Members[i].Ordinal = &v
	}
	out := Project(model.ProjectionRequest{InvestigationID: g.RootID}, g.GroupScope, nodes, edges, []model.Group{g, h, seq})
	if len(out.Nodes) != 2 || len(out.Annotations) != 1 || out.Annotations[0].MemberNodeIDs[0] != nodes[1].ID {
		t.Fatalf("overlap or sequence ordering: %+v", out)
	}
}
