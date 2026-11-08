package psql

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

func TestAgentImportWithoutSomIssue(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	project := softDeleteProjectID()
	groupCleanup(t, db, project)
	inv := groupCase(t, db, project, nil)
	actor := uuid.NewString()
	at := time.Now().UTC()
	eventRef, entityRef := "event", "entity"
	provenance := model.GatewayProvenance{Source: "pt-maxpatrol-siem", ExternalID: uuid.NewString(), FetchedAt: at}
	request := model.ImportRequest{
		ProjectID: project, InvestigationID: inv, Origin: "agent", Actor: actor,
		Selection: model.GatewaySelection{
			Events:   []model.GatewayEvent{{SnapshotID: eventRef, Direct: true, Title: "Selected event", EventType: "process", OccurredAt: at, Provenance: provenance}},
			Entities: []model.GatewayEntity{{SnapshotID: entityRef, TypeCode: "host", Value: "agent-regression.local", Provenance: []model.GatewayProvenance{{Source: provenance.Source, ExternalID: "host:agent-regression.local", FetchedAt: at}}}},
		},
		Nodes: []model.AgentNode{
			{Ref: eventRef, SnapshotEventID: &eventRef, Why: "Event matches the task"},
			{Ref: entityRef, SnapshotEntityID: &entityRef, Why: "Host named in the task"},
		},
		Edges: []model.AgentEdge{{SourceRef: eventRef, TargetRef: entityRef, RelationCode: "mentions", Why: "Event mentions the host", EvidenceEventRefs: []string{eventRef}}},
	}
	stats, err := db.ImportContext(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Events != 1 || stats.Entities != 1 || stats.Nodes != 2 || stats.Edges != 1 {
		t.Fatalf("unexpected write counts: %+v", stats)
	}
	nodes, err := db.GraphNodes(ctx, project, inv, model.NodeFilter{})
	if err != nil || len(nodes) != 2 {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	for _, node := range nodes {
		if len(node.SomIssueIDs) != 0 || node.Origin != "agent" {
			t.Fatalf("unexpected node provenance: %+v", node)
		}
	}
	edges, err := db.GraphEdges(ctx, project, inv, model.EdgeFilter{})
	if err != nil || len(edges) != 1 || edges[0].Origin != "agent" || edges[0].Status != "proposed" || edges[0].OriginRef == nil || *edges[0].OriginRef != actor {
		t.Fatalf("edge provenance: %+v %v", edges, err)
	}
	legacyIssue := uuid.NewString()
	if _, err = db.Pgx().Exec(ctx, `INSERT INTO graph_node_som_issues(graph_node_id,som_issue_id) VALUES($1::uuid,$2::uuid)`, nodes[0].ID, legacyIssue); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ImportContext(ctx, request); err != nil {
		t.Fatal(err)
	}
	reloaded, err := db.GraphNodes(ctx, project, inv, model.NodeFilter{})
	if err != nil || len(reloaded) != 2 {
		t.Fatalf("replay: %+v %v", reloaded, err)
	}
	for _, node := range reloaded {
		if node.ID == nodes[0].ID && !slices.Contains(node.SomIssueIDs, legacyIssue) {
			t.Fatal("replay removed historical SoM provenance")
		}
	}
	request.ProjectID = "ffffffffffff"
	if _, err = db.ImportContext(ctx, request); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("foreign project: %v", err)
	}
}
