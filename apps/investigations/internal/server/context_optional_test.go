package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

func TestRootOnlyFindingKeepsMetadataWithoutDanglingEntityReferences(t *testing.T) {
	var response gatewayclient.ResolveContextResponse
	if err := json.Unmarshal([]byte(`{"findings":[{"ref":{"source_code":"pt-nad","source_instance":"19","record_type":"nad_attack","external_id":"attack-1"},"kind":"nad_attack","title":"Attack","entities":[{"type":"ip","value":"10.0.0.1","roles":["src"]}]}]}`), &response); err != nil {
		t.Fatal(err)
	}
	resolve := false
	refs := []gatewayclient.SourceObjectRef{response.Findings[0].Ref}
	result, err := convertGatewayContext(response, gatewayclient.ResolveContextRequest{Findings: &refs, ExpandFindings: &resolve})
	if err != nil {
		t.Fatal(err)
	}
	finding := result.Selection.Findings[0]
	if len(finding.EntitySnapshotIDs) != 0 || len(result.Selection.Entities) != 0 {
		t.Fatalf("dangling participant references: %+v", result.Selection)
	}
	if !bytes.Contains(finding.Normalized, []byte("10.0.0.1")) {
		t.Fatal("participant metadata lost")
	}
}

func TestContextImportForwardsResolve(t *testing.T) {
	for _, mode := range []string{"omitted", "false", "true"} {
		for _, hypothesis := range []bool{false, true} {
			t.Run(mode+"/hypothesis="+map[bool]string{false: "no", true: "yes"}[hypothesis], func(t *testing.T) {
				var body investigations.ContextSelection
				if err := json.Unmarshal([]byte(`{"findings":[{"source_code":"pt-maxpatrol-siem","record_type":"siem_incident","external_id":"11111111-1111-4111-8111-111111111111","time_range":{"from":"2026-09-01T00:00:00Z","to":"2026-09-02T00:00:00Z"}}],"events":[],"sessions":[],"entities":[]}`), &body); err != nil {
					t.Fatal(err)
				}
				if mode != "omitted" {
					value := mode == "true"
					body.ExpandFindings = &value
				}
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api/v1/context/resolve" || r.Header.Get("X-Project-ID") != "aabbccddee" {
						t.Errorf("unexpected Gateway request")
					}
					var request gatewayclient.ResolveContextRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if (request.ExpandFindings == nil) != (body.ExpandFindings == nil) || (request.ExpandFindings != nil && *request.ExpandFindings != *body.ExpandFindings) {
						t.Errorf("expand_findings was not forwarded")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"findings":[{"ref":{"source_code":"pt-maxpatrol-siem","record_type":"siem_incident","external_id":"11111111-1111-4111-8111-111111111111","time_range":{"from":"2026-09-01T00:00:00Z","to":"2026-09-02T00:00:00Z"}},"kind":"siem_incident","title":"Incident","severity":"high","occurred_at":"2026-09-01T12:00:00Z","fetched_at":"2026-09-01T12:00:00Z"}],"events":[],"entities":[],"sessions":[],"relations":[],"resolutions":[],"source_errors":[]}`))
				}))
				defer gateway.Close()
				server := &Server{gateway: gatewayclient.New(gatewayclient.Config{BaseURL: gateway.URL})}
				ctx := socctx.WithScope(context.Background(), socctx.Scope{ProjectID: "aabbccddee"})
				id := uuid.New()
				var imported model.ImportRequest
				if hypothesis {
					db := &hypothesisFakeDB{hypothesis: model.Hypothesis{Status: "open"}}
					server.db = db
					_, err := server.AddHypothesisContext(ctx, investigations.AddHypothesisContextRequestObject{InvestigationId: id, HypothesisId: uuid.New(), Body: &body})
					if err != nil {
						t.Fatal(err)
					}
					imported = db.lastImport
				} else {
					db := &mcpRecordingDB{}
					server.db = db
					_, err := server.AddInvestigationContext(ctx, investigations.AddInvestigationContextRequestObject{InvestigationId: id, Body: &body})
					if err != nil {
						t.Fatal(err)
					}
					imported = db.request
				}
				if len(imported.Selection.Findings) != 1 || !imported.Selection.Findings[0].Direct || len(imported.Selection.Events)+len(imported.Selection.Entities)+len(imported.Selection.Sessions) != 0 {
					t.Fatalf("unexpected import: %+v", imported)
				}
			})
		}
	}
}
