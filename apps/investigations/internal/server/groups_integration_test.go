package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store/psql"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

// Real HTTP + JWT + MCP + PostgreSQL, using isolated synthetic evidence.
// No production credentials, shared investigations or live NAD are involved.
func TestGroupingHTTPAndMCPRuntime(t *testing.T) {
	uri := os.Getenv("INVESTIGATIONS_TEST_DATABASE_URI")
	if uri == "" {
		t.Skip("INVESTIGATIONS_TEST_DATABASE_URI is not set")
	}
	ctx := context.Background()
	db, err := psql.New(ctx, uri, 4, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	defer func() {
		for _, table := range []string{"investigations", "events", "entities"} {
			if _, err := db.Pgx().Exec(ctx, `DELETE FROM `+table+` WHERE project_id=$1`, project); err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	}()
	inv, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: project, Title: "runtime group case"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: project, Title: "independent runtime case"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	selection := model.GatewaySelection{}
	for i := 0; i < 2; i++ {
		selection.Events = append(selection.Events, model.GatewayEvent{SnapshotID: fmt.Sprint(i), Direct: true, Title: "test event", EventType: "network", OccurredAt: at, Provenance: model.GatewayProvenance{Source: "pt-nad", ExternalID: fmt.Sprint("runtime:", i), FetchedAt: at}})
	}
	for _, id := range []string{inv.ID, second.ID} {
		if _, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: id, Origin: "analyst", Selection: selection}); err != nil {
			t.Fatal(err)
		}
	}
	nodes, err := db.GraphNodes(ctx, project, inv.ID, model.NodeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.ServerConfig{Auth: config.AuthConfig{AccessTokenPublicKey: pub, AccessTokenIssuer: "grouping-test", AccessTokenAudience: "test-api", AccessTokenKid: "test-key", AccessTokenTyp: "access+jwt"}}
	claims := coreauth.AccessTokenClaims{SessionID: "test-session", SubjectKind: "user", ClientID: "test-client", RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Auth.AccessTokenIssuer, Subject: "test-reviewer", Audience: jwt.ClaimStrings{cfg.Auth.AccessTokenAudience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now()), ID: uuid.NewString()}}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = cfg.Auth.AccessTokenKid
	token.Header["typ"] = cfg.Auth.AccessTokenTyp
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := httptest.NewServer(transport.NewHandler(transport.Dependencies{Cfg: cfg, Log: log, Server: New(db, log, nil, nil, nil)}))
	defer api.Close()
	call := func(method, path string, body any, projectID string, authenticated bool, expected int) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			var err error
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequest(method, api.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("X-Project-ID", projectID)
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+signed)
		}
		response, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != expected {
			t.Fatalf("%s %s: status=%d expected=%d body=%s", method, path, response.StatusCode, expected, data)
		}
		if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "data: ") {
					return []byte(strings.TrimPrefix(line, "data: "))
				}
			}
			t.Fatalf("MCP SSE response has no data: %s", data)
		}
		return data
	}
	base := "/api/v1/investigations/" + inv.ID
	call("GET", base+"/graph/projection", nil, project, false, 401)
	call("GET", base+"/graph/projection", nil, "ffffffffffff", true, 404)
	call("GET", base+"/graph/projection?min_confidence=2", nil, project, true, 422)
	payload := map[string]any{"som_issue_ids": []string{uuid.NewString()}, "events": []any{}, "entities": []any{}, "edges": []any{}, "nodes": []any{map[string]any{"ref": "a", "why": "primary event in the same trace", "event_id": *nodes[0].EventID}, map[string]any{"ref": "b", "why": "duplicate event in the same trace", "event_id": *nodes[1].EventID}}, "event_group_proposals": []any{map[string]any{"proposal_id": uuid.NewString(), "kind": "same_event", "title": "same occurrence", "why": "same trace reference", "evidence_event_refs": []string{"a", "b"}, "members": []any{map[string]any{"node_ref": "a", "role": "primary"}, map[string]any{"node_ref": "b", "role": "duplicate"}}}}}
	raw := call("POST", base+"/agent-results", payload, project, true, 201)
	var imported struct {
		Groups []model.GroupImportResult `json:"groups"`
	}
	if err := json.Unmarshal(raw, &imported); err != nil || len(imported.Groups) != 1 {
		t.Fatalf("agent group result: %s %v", raw, err)
	}
	groupPath := base + "/event-groups/" + imported.Groups[0].GroupID
	var g model.Group
	if err := json.Unmarshal(call("GET", groupPath, nil, project, true, 200), &g); err != nil {
		t.Fatal(err)
	}
	for _, m := range g.Members {
		if m.Status != "proposed" {
			t.Fatal("HTTP proposal auto-confirmed")
		}
	}
	hidden := call("GET", "/api/v1/investigations/"+second.ID+"/event-groups/"+g.ID, nil, project, true, 404)
	missing := call("GET", base+"/event-groups/"+uuid.NewString(), nil, project, true, 404)
	if !bytes.Equal(hidden, missing) {
		t.Fatal("foreign and missing groups distinguishable")
	}
	review := model.GroupReview{OperationID: uuid.NewString(), Version: g.Version, Reason: "analyst verified"}
	for _, m := range g.Members {
		review.Members = append(review.Members, model.GroupReviewMember{ID: m.ID, Version: m.Version, Status: "confirmed"})
	}
	call("POST", "/api/v1/investigations/"+second.ID+"/event-groups/"+g.ID+"/review", review, project, true, 404)
	accepted := call("POST", groupPath+"/review", review, project, true, 200)
	replayed := call("POST", groupPath+"/review", review, project, true, 200)
	if !bytes.Equal(accepted, replayed) {
		t.Fatal("HTTP idempotent replay changed response")
	}
	review.OperationID = uuid.NewString()
	call("POST", groupPath+"/review", review, project, true, 409)
	history := call("GET", groupPath+"/history?limit=1", nil, project, true, 200)
	if !bytes.Contains(history, []byte("test-reviewer")) || !bytes.Contains(history, []byte("next_cursor")) {
		t.Fatal("missing authenticated audit actor or cursor")
	}
	httpProjection := call("GET", base+"/graph/projection", nil, project, true, 200)
	var projected model.GraphProjection
	if err := json.Unmarshal(httpProjection, &projected); err != nil || len(projected.Nodes) != 1 || len(projected.RawNodes) != 2 {
		t.Fatalf("HTTP collapse failed: %s %v", httpProjection, err)
	}
	mcpBody := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "get_investigation_graph", "arguments": map[string]any{"investigation_id": inv.ID, "projection": "grouped"}}}
	mcpResponse := call("POST", "/mcp", mcpBody, project, true, 200)
	var mcpResult struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
			IsError           bool            `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(mcpResponse, &mcpResult); err != nil || mcpResult.Result.IsError {
		t.Fatalf("MCP response: %s %v", mcpResponse, err)
	}
	var mcpProjection model.GraphProjection
	if err := json.Unmarshal(mcpResult.Result.StructuredContent, &mcpProjection); err != nil || !reflect.DeepEqual(projected, mcpProjection) {
		t.Fatalf("HTTP/MCP projection diverged: %s %v", mcpResponse, err)
	}
	rawGraph := call("GET", base+"/graph", nil, project, true, 200)
	if bytes.Contains(rawGraph, []byte("event-group:")) {
		t.Fatal("raw graph contract changed")
	}
	// MCP proposals use the same transaction and stay proposed in a separate root.
	payload["investigation_id"] = second.ID
	proposal := payload["event_group_proposals"].([]any)[0].(map[string]any)
	proposal["proposal_id"] = uuid.NewString()
	mcpBody["params"] = map[string]any{"name": "add_investigation_agent_results", "arguments": payload}
	raw = call("POST", "/mcp", mcpBody, project, true, 200)
	if err := json.Unmarshal(raw, &mcpResult); err != nil || mcpResult.Result.IsError {
		t.Fatalf("MCP proposal: %s %v", raw, err)
	}
	var result struct {
		Groups []model.GroupImportResult `json:"groups"`
	}
	if err := json.Unmarshal(mcpResult.Result.StructuredContent, &result); err != nil || len(result.Groups) != 1 || result.Groups[0].GroupID == g.ID {
		t.Fatalf("MCP proposal scope: %s %v", raw, err)
	}
	call("POST", "/mcp", mcpBody, project, false, 401)
}
