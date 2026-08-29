package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	gatewaycontract "github.com/sb0rka/ir/packages/contract/gateway"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/hypotheses"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

type hypothesisFakeDB struct {
	store.Database
	hypothesis model.Hypothesis
	list       []model.Hypothesis
	projection model.HypothesisGraph
	node       model.GraphNode
	edge       model.GraphEdge
	err        error
	lastPatch  model.HypothesisPatch
	lastImport model.ImportRequest
	calls      []string
}

func (f *hypothesisFakeDB) CreateHypothesis(_ context.Context, input model.HypothesisNew) (model.Hypothesis, error) {
	f.calls = append(f.calls, "create")
	if f.err != nil {
		return model.Hypothesis{}, f.err
	}
	out := f.hypothesis
	out.ProjectID, out.InvestigationID, out.Statement = input.ProjectID, input.InvestigationID, input.Statement
	out.Description = input.Description
	return out, nil
}

func (f *hypothesisFakeDB) ListHypotheses(context.Context, string, string, model.HypothesisFilter) ([]model.Hypothesis, error) {
	f.calls = append(f.calls, "list")
	return f.list, f.err
}

func (f *hypothesisFakeDB) GetHypothesis(context.Context, string, string, string) (model.Hypothesis, error) {
	f.calls = append(f.calls, "get")
	return f.hypothesis, f.err
}

func (f *hypothesisFakeDB) UpdateHypothesis(_ context.Context, patch model.HypothesisPatch) (model.Hypothesis, error) {
	f.calls = append(f.calls, "update")
	f.lastPatch = patch
	if f.err != nil {
		return model.Hypothesis{}, f.err
	}
	out := f.hypothesis
	out.Version++
	if patch.Statement != nil {
		out.Statement = *patch.Statement
	}
	return out, nil
}

func (f *hypothesisFakeDB) DeleteHypothesis(context.Context, string, string, string) error {
	f.calls = append(f.calls, "delete")
	return f.err
}

func (f *hypothesisFakeDB) HypothesisGraph(context.Context, string, string, string, model.EdgeFilter) (model.HypothesisGraph, error) {
	f.calls = append(f.calls, "graph")
	return f.projection, f.err
}

func (f *hypothesisFakeDB) AddHypothesisNode(context.Context, string, string, string, string) error {
	f.calls = append(f.calls, "add-node")
	return f.err
}

func (f *hypothesisFakeDB) DeleteHypothesisNode(context.Context, string, string, string, string) error {
	f.calls = append(f.calls, "delete-node")
	return f.err
}

func (f *hypothesisFakeDB) AddHypothesisEdge(context.Context, string, string, string, string) error {
	f.calls = append(f.calls, "add-edge")
	return f.err
}

func (f *hypothesisFakeDB) DeleteHypothesisEdge(context.Context, string, string, string, string) error {
	f.calls = append(f.calls, "delete-edge")
	return f.err
}

func (f *hypothesisFakeDB) CreateHypothesisNode(context.Context, string, string, string, string, *string, *string, string, []string) (model.GraphNode, error) {
	f.calls = append(f.calls, "create-node")
	return f.node, f.err
}

func (f *hypothesisFakeDB) CreateHypothesisGraphEdge(context.Context, string, model.GraphEdgeNew) (model.GraphEdge, error) {
	f.calls = append(f.calls, "create-edge")
	return f.edge, f.err
}

func (f *hypothesisFakeDB) ImportContext(_ context.Context, input model.ImportRequest) (model.ImportStats, error) {
	f.calls = append(f.calls, "import")
	f.lastImport = input
	return model.ImportStats{Events: 1, Nodes: 1, Warnings: []string{}}, f.err
}

func TestHypothesisHandlers(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	investigationID := uuid.New()
	hypothesisID := uuid.New()
	nodeID := uuid.New()
	targetID := uuid.New()
	edgeID := uuid.New()
	fake := &hypothesisFakeDB{
		hypothesis: model.Hypothesis{
			ID: hypothesisID.String(), ProjectID: "aabbccddee", InvestigationID: investigationID.String(),
			Statement: "Initial statement", Status: "proposed", Origin: "analyst", Version: 1,
			CreatedAt: now, UpdatedAt: now,
		},
		node: model.GraphNode{
			ID: nodeID.String(), InvestigationID: investigationID.String(), NodeType: "entity",
			EntityID: stringPointerForServer(uuid.NewString()), Origin: "analyst", CreatedAt: now,
		},
		edge: model.GraphEdge{
			ID: edgeID.String(), InvestigationID: investigationID.String(), SourceNodeID: nodeID.String(),
			TargetNodeID: targetID.String(), RelationCode: "connected_to", Status: "confirmed",
			Origin: "analyst", Metadata: []byte(`{}`), Version: 1, CreatedAt: now, UpdatedAt: now,
		},
	}
	fake.list = []model.Hypothesis{fake.hypothesis}
	fake.projection = model.HypothesisGraph{Nodes: []model.GraphNode{fake.node}, Edges: []model.GraphEdge{fake.edge}}
	s := &Server{db: fake}
	ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "aabbccddee"})

	createBody := hypotheses.HypothesisCreate{Statement: "  Initial statement  "}
	created, err := s.CreateHypothesis(ctx, hypotheses.CreateHypothesisRequestObject{
		InvestigationId: investigationID, Body: &createBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := hypotheses.Hypothesis(created.(hypotheses.CreateHypothesis201JSONResponse)); got.Statement != "Initial statement" || got.ProjectId != "aabbccddee" {
		t.Fatalf("created=%#v", got)
	}

	listed, err := s.ListHypotheses(ctx, hypotheses.ListHypothesesRequestObject{InvestigationId: investigationID})
	if err != nil || len(hypotheses.HypothesisPage(listed.(hypotheses.ListHypotheses200JSONResponse)).Hypotheses) != 1 {
		t.Fatalf("list response=%#v err=%v", listed, err)
	}
	got, err := s.GetHypothesis(ctx, hypotheses.GetHypothesisRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID})
	if err != nil || hypotheses.Hypothesis(got.(hypotheses.GetHypothesis200JSONResponse)).Id != hypothesisID {
		t.Fatalf("get response=%#v err=%v", got, err)
	}

	statement := "  Revised statement  "
	patchBody := hypotheses.HypothesisPatch{Version: 1, Statement: &statement}
	updated, err := s.UpdateHypothesis(ctx, hypotheses.UpdateHypothesisRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &patchBody,
	})
	if err != nil || hypotheses.Hypothesis(updated.(hypotheses.UpdateHypothesis200JSONResponse)).Statement != "Revised statement" || fake.lastPatch.Statement == nil || *fake.lastPatch.Statement != "Revised statement" {
		t.Fatalf("update response=%#v patch=%#v err=%v", updated, fake.lastPatch, err)
	}

	if response, err := s.GetHypothesisGraph(ctx, graph.GetHypothesisGraphRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID,
	}); err != nil || len(graph.HypothesisGraph(response.(graph.GetHypothesisGraph200JSONResponse)).Edges) != 1 {
		t.Fatalf("graph response=%#v err=%v", response, err)
	}
	nodeBody := graph.NodeCreate{NodeType: graph.Entity, EntityId: uuidPointer(*fake.node.EntityID)}
	if response, err := s.CreateHypothesisNode(ctx, graph.CreateHypothesisNodeRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &nodeBody,
	}); err != nil || graph.GraphNode(response.(graph.CreateHypothesisNode201JSONResponse)).Id != nodeID {
		t.Fatalf("node response=%#v err=%v", response, err)
	}
	edgeBody := graph.GraphEdgeCreate{SourceNodeId: nodeID, TargetNodeId: targetID, RelationCode: "connected_to"}
	if response, err := s.CreateHypothesisGraphEdge(ctx, graph.CreateHypothesisGraphEdgeRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &edgeBody,
	}); err != nil || graph.GraphEdge(response.(graph.CreateHypothesisGraphEdge201JSONResponse)).Id != edgeID {
		t.Fatalf("edge response=%#v err=%v", response, err)
	}

	membershipCalls := []struct {
		name string
		call func() error
	}{
		{"add node", func() error {
			_, err := s.AddHypothesisNode(ctx, hypotheses.AddHypothesisNodeRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID, NodeId: nodeID})
			return err
		}},
		{"delete node", func() error {
			_, err := s.DeleteHypothesisNode(ctx, hypotheses.DeleteHypothesisNodeRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID, NodeId: nodeID})
			return err
		}},
		{"add edge", func() error {
			_, err := s.AddHypothesisEdge(ctx, hypotheses.AddHypothesisEdgeRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID, EdgeId: edgeID})
			return err
		}},
		{"delete edge", func() error {
			_, err := s.DeleteHypothesisEdge(ctx, hypotheses.DeleteHypothesisEdgeRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID, EdgeId: edgeID})
			return err
		}},
		{"delete hypothesis", func() error {
			_, err := s.DeleteHypothesis(ctx, hypotheses.DeleteHypothesisRequestObject{InvestigationId: investigationID, HypothesisId: hypothesisID})
			return err
		}},
	}
	for _, test := range membershipCalls {
		if err := test.call(); err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
	}

	fake.hypothesis.Status = "active"
	agentBody := investigations.AgentResultBatch{
		Edges: []investigations.AgentEdge{}, Entities: []investigations.AgentEntitySelection{},
		Events: []investigations.AgentEventSelection{}, Nodes: []investigations.AgentNode{},
		SomIssueIds: []uuid.UUID{uuid.New()},
	}
	if _, err := s.AddHypothesisAgentResults(ctx, investigations.AddHypothesisAgentResultsRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &agentBody,
	}); err != nil {
		t.Fatal(err)
	}
	if fake.lastImport.HypothesisID == nil || *fake.lastImport.HypothesisID != hypothesisID.String() || !fake.lastImport.RequireActiveHypothesis {
		t.Fatalf("agent import scope=%#v", fake.lastImport)
	}
}

func TestHypothesisContextHandlerAndResolvedWrites(t *testing.T) {
	t.Parallel()
	investigationID := uuid.New()
	hypothesisID := uuid.New()
	now := time.Now().UTC()
	fake := &hypothesisFakeDB{hypothesis: model.Hypothesis{
		ID: hypothesisID.String(), ProjectID: "aabbccddee", InvestigationID: investigationID.String(),
		Statement: "Check source", Status: "active", Origin: "analyst", Version: 2,
		CreatedAt: now, UpdatedAt: now,
	}}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/context/resolve" || r.Header.Get("X-Project-ID") != "aabbccddee" {
			t.Fatalf("unexpected Gateway request: %s project=%s", r.URL.Path, r.Header.Get("X-Project-ID"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gatewaycontract.ResolveContextResponse{
			Entities: []gatewaycontract.Entity{}, Findings: []gatewaycontract.Finding{}, Relations: []gatewaycontract.Relation{},
			Resolutions: []gatewaycontract.ObjectResolution{}, Sessions: []gatewaycontract.Session{}, SourceErrors: []gatewaycontract.SourceError{},
			Events: []gatewaycontract.Event{{
				Attributes: map[string]any{}, Entities: []gatewaycontract.EntityMention{}, FetchedAt: now, OccurredAt: now,
				Severity: gatewaycontract.EventSeverity("medium"), SourceCode: "pt-maxpatrol-siem",
				SourceEventId: "source-event", Title: "Resolved event", Type: "test",
			}},
		})
	}))
	defer gateway.Close()
	s := &Server{db: fake, gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL, HTTPClient: gateway.Client()})}
	ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "aabbccddee"})
	body := investigations.ContextSelection{
		Entities: []investigations.EntitySourceRef{}, Findings: []investigations.SourceObjectRef{}, Sessions: []investigations.SourceObjectRef{},
		Events: []investigations.EventSourceRef{{SourceCode: "pt-maxpatrol-siem", SourceEventId: "source-event"}},
	}
	if _, err := s.AddHypothesisContext(ctx, investigations.AddHypothesisContextRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &body,
	}); err != nil {
		t.Fatal(err)
	}
	if fake.lastImport.HypothesisID == nil || *fake.lastImport.HypothesisID != hypothesisID.String() || fake.lastImport.RequireActiveHypothesis {
		t.Fatalf("context import scope=%#v", fake.lastImport)
	}

	fake.hypothesis.Status = "resolved"
	_, err := s.AddHypothesisContext(ctx, investigations.AddHypothesisContextRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, Body: &body,
	})
	var domain *httperr.Error
	if !errorsAs(err, &domain) || domain.Status != http.StatusConflict {
		t.Fatalf("resolved context error=%v", err)
	}
	fake.err = &store.ConflictError{IDs: []string{hypothesisID.String()}}
	_, err = s.AddHypothesisNode(ctx, hypotheses.AddHypothesisNodeRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID, NodeId: uuid.New(),
	})
	if !errorsAs(err, &domain) || domain.Status != http.StatusConflict {
		t.Fatalf("resolved membership error=%v", err)
	}
}

func TestHypothesisFilterValidation(t *testing.T) {
	t.Parallel()
	s := &Server{db: &hypothesisFakeDB{}}
	ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "aabbccddee"})
	investigationID, hypothesisID := uuid.New(), uuid.New()

	badHypothesisStatus := hypotheses.HypothesisStatus("unknown")
	_, err := s.ListHypotheses(ctx, hypotheses.ListHypothesesRequestObject{
		InvestigationId: investigationID,
		Params:          hypotheses.ListHypothesesParams{Status: &badHypothesisStatus},
	})
	var domain *httperr.Error
	if !errorsAs(err, &domain) || domain.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid hypothesis status error=%v", err)
	}

	badEdgeStatuses := []graph.GraphEdgeStatus{graph.GraphEdgeStatus("unknown")}
	_, err = s.GetHypothesisGraph(ctx, graph.GetHypothesisGraphRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID,
		Params: graph.GetHypothesisGraphParams{Statuses: &badEdgeStatuses},
	})
	if !errorsAs(err, &domain) || domain.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid edge status error=%v", err)
	}

	invalidConfidence := float32(1.1)
	_, err = s.GetHypothesisGraph(ctx, graph.GetHypothesisGraphRequestObject{
		InvestigationId: investigationID, HypothesisId: hypothesisID,
		Params: graph.GetHypothesisGraphParams{MinConfidence: &invalidConfidence},
	})
	if !errorsAs(err, &domain) || domain.Status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid min_confidence error=%v", err)
	}
}

func stringPointerForServer(value string) *string { return &value }

func uuidPointer(value string) *uuid.UUID {
	id := uuid.MustParse(value)
	return &id
}

func errorsAs(err error, target any) bool {
	return err != nil && errors.As(err, target)
}
