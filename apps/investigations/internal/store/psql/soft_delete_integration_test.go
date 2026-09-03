package psql

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
)

func softDeleteTestDB(t *testing.T) *DB {
	t.Helper()
	uri := os.Getenv("INVESTIGATIONS_TEST_DATABASE_URI")
	if uri == "" {
		t.Skip("INVESTIGATIONS_TEST_DATABASE_URI is not set")
	}
	db, err := New(context.Background(), uri, 4, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func softDeleteProjectID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func cleanupSoftDeleteProject(t *testing.T, db *DB, projectID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := db.Pgx().Exec(ctx, `DELETE FROM investigations WHERE project_id=$1`, projectID); err != nil {
			t.Errorf("cleanup investigations: %v", err)
		}
		if _, err := db.Pgx().Exec(ctx, `DELETE FROM entities WHERE project_id=$1`, projectID); err != nil {
			t.Errorf("cleanup entities: %v", err)
		}
	})
}

func TestDeleteInvestigationSoftDeletesSubtree(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	projectID := softDeleteProjectID()
	cleanupSoftDeleteProject(t, db, projectID)

	root, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: projectID, Title: "root"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: projectID, Title: "child", ParentID: &root.ID})
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: projectID, Title: "neighbor"})
	if err != nil {
		t.Fatal(err)
	}
	hypothesis, err := db.CreateHypothesis(ctx, model.HypothesisNew{
		ProjectID: projectID, InvestigationID: child.ID, Statement: "child hypothesis",
	})
	if err != nil {
		t.Fatal(err)
	}
	entity, err := db.CreateEntity(ctx, model.EntityNew{
		ProjectID: projectID, InvestigationID: child.ID, TypeCode: "host", CanonicalKey: "soft-delete-host",
	})
	if err != nil {
		t.Fatal(err)
	}
	var nodeID string
	if err := db.Pgx().QueryRow(ctx, `SELECT id::text FROM graph_nodes WHERE investigation_id=$1::uuid AND entity_id=$2::uuid`, child.ID, entity.ID).Scan(&nodeID); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteInvestigation(ctx, "ffffffffffff", root.ID); !errors.Is(err, store.ErrInvestigationNotFound) {
		t.Fatalf("foreign project delete error=%v", err)
	}
	if _, err := db.GetInvestigation(ctx, projectID, root.ID); err != nil {
		t.Fatalf("foreign project delete changed root: %v", err)
	}

	if err := db.DeleteInvestigation(ctx, projectID, root.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{root.ID, child.ID} {
		if _, err := db.GetInvestigation(ctx, projectID, id); !errors.Is(err, store.ErrInvestigationNotFound) {
			t.Fatalf("get deleted investigation %s: %v", id, err)
		}
	}
	exists, err := db.InvestigationExists(ctx, projectID, root.ID)
	if err != nil || exists {
		t.Fatalf("deleted investigation exists=%v err=%v", exists, err)
	}
	listed, err := db.ListInvestigations(ctx, projectID, model.InvestigationFilter{RootsOnly: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != neighbor.ID {
		t.Fatalf("listed investigations=%#v", listed)
	}
	if _, err := db.UpdateInvestigation(ctx, model.InvestigationPatch{ProjectID: projectID, InvestigationID: root.ID, Version: root.Version}); !errors.Is(err, store.ErrInvestigationNotFound) {
		t.Fatalf("update deleted investigation error=%v", err)
	}
	if _, err := db.GetInvestigation(ctx, projectID, neighbor.ID); err != nil {
		t.Fatalf("neighbor hidden by delete: %v", err)
	}
	if _, err := db.GetHypothesis(ctx, projectID, child.ID, hypothesis.ID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("get deleted hypothesis: %v", err)
	}
	if _, err := db.GetNode(ctx, projectID, child.ID, nodeID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("get graph node of deleted investigation: %v", err)
	}

	var deletedInvestigations, deletedHypotheses, retainedNodes int
	if err := db.Pgx().QueryRow(ctx, `SELECT count(*)::int FROM investigations WHERE project_id=$1 AND id=ANY($2::uuid[]) AND is_deleted=true`, projectID, []string{root.ID, child.ID}).Scan(&deletedInvestigations); err != nil {
		t.Fatal(err)
	}
	if err := db.Pgx().QueryRow(ctx, `SELECT count(*)::int FROM hypotheses WHERE project_id=$1 AND id=$2::uuid AND is_deleted=true`, projectID, hypothesis.ID).Scan(&deletedHypotheses); err != nil {
		t.Fatal(err)
	}
	if err := db.Pgx().QueryRow(ctx, `SELECT count(*)::int FROM graph_nodes WHERE id=$1::uuid`, nodeID).Scan(&retainedNodes); err != nil {
		t.Fatal(err)
	}
	if deletedInvestigations != 2 || deletedHypotheses != 1 || retainedNodes != 1 {
		t.Fatalf("deleted investigations=%d hypotheses=%d retained nodes=%d", deletedInvestigations, deletedHypotheses, retainedNodes)
	}
	if err := db.DeleteInvestigation(ctx, projectID, root.ID); !errors.Is(err, store.ErrInvestigationNotFound) {
		t.Fatalf("repeated delete error=%v", err)
	}
	if _, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: projectID, Title: "orphan", ParentID: &root.ID}); !errors.Is(err, store.ErrParentNotFound) {
		t.Fatalf("create below deleted parent error=%v", err)
	}
	if _, err := db.CreateHypothesis(ctx, model.HypothesisNew{ProjectID: projectID, InvestigationID: child.ID, Statement: "hidden"}); !errors.Is(err, store.ErrInvestigationNotFound) {
		t.Fatalf("create hypothesis below deleted investigation error=%v", err)
	}
}

func TestDeleteHypothesisSoftDeletesOnlyTarget(t *testing.T) {
	db := softDeleteTestDB(t)
	ctx := context.Background()
	projectID := softDeleteProjectID()
	cleanupSoftDeleteProject(t, db, projectID)

	investigation, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: projectID, Title: "case"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := db.CreateHypothesis(ctx, model.HypothesisNew{ProjectID: projectID, InvestigationID: investigation.ID, Statement: "target"})
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := db.CreateHypothesis(ctx, model.HypothesisNew{ProjectID: projectID, InvestigationID: investigation.ID, Statement: "neighbor"})
	if err != nil {
		t.Fatal(err)
	}
	entity, err := db.CreateEntity(ctx, model.EntityNew{
		ProjectID: projectID, InvestigationID: investigation.ID, TypeCode: "host", CanonicalKey: "soft-delete-hypothesis-host",
	})
	if err != nil {
		t.Fatal(err)
	}
	var nodeID string
	if err := db.Pgx().QueryRow(ctx, `SELECT id::text FROM graph_nodes WHERE investigation_id=$1::uuid AND entity_id=$2::uuid`, investigation.ID, entity.ID).Scan(&nodeID); err != nil {
		t.Fatal(err)
	}
	if err := db.AddHypothesisNode(ctx, projectID, investigation.ID, target.ID, nodeID); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteHypothesis(ctx, "ffffffffffff", investigation.ID, target.ID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("foreign project delete error=%v", err)
	}
	if err := db.DeleteHypothesis(ctx, projectID, investigation.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetHypothesis(ctx, projectID, investigation.ID, target.ID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("get deleted hypothesis: %v", err)
	}
	if _, err := db.UpdateHypothesis(ctx, model.HypothesisPatch{ProjectID: projectID, InvestigationID: investigation.ID, HypothesisID: target.ID, Version: target.Version}); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("update deleted hypothesis: %v", err)
	}
	if _, err := db.HypothesisGraph(ctx, projectID, investigation.ID, target.ID, model.EdgeFilter{}); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("graph of deleted hypothesis: %v", err)
	}
	if err := db.AddHypothesisNode(ctx, projectID, investigation.ID, target.ID, nodeID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("membership write on deleted hypothesis: %v", err)
	}
	items, err := db.ListHypotheses(ctx, projectID, investigation.ID, model.HypothesisFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != neighbor.ID {
		t.Fatalf("listed hypotheses=%#v", items)
	}
	var retainedMemberships int
	if err := db.Pgx().QueryRow(ctx, `SELECT count(*)::int FROM hypothesis_nodes WHERE hypothesis_id=$1::uuid AND investigation_id=$2::uuid`, target.ID, investigation.ID).Scan(&retainedMemberships); err != nil {
		t.Fatal(err)
	}
	if retainedMemberships != 1 {
		t.Fatalf("retained memberships=%d", retainedMemberships)
	}
	if err := db.DeleteHypothesis(ctx, projectID, investigation.ID, target.ID); !errors.Is(err, store.ErrRecordNotFound) {
		t.Fatalf("repeated delete error=%v", err)
	}
}

func TestActivePartialIndexesExist(t *testing.T) {
	db := softDeleteTestDB(t)
	rows, err := db.Pgx().Query(context.Background(), `SELECT indexname,indexdef FROM pg_indexes
		WHERE schemaname=current_schema() AND indexname=ANY($1::text[])`, []string{
		"ix_investigations_project_created_at",
		"ix_investigations_parent",
		"ix_investigations_status",
		"ix_hypotheses_project_investigation_created",
		"ix_hypotheses_project_investigation_status",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(definition, "WHERE (is_deleted = false)") {
			t.Fatalf("index %s is not partial: %s", name, definition)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 5 {
		t.Fatalf("found %d active partial indexes, want 5", seen)
	}
}
