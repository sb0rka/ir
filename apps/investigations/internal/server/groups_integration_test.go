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
	child, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: project, ParentID: &inv.ID, Title: "runtime child case"})
	if err != nil {
		t.Fatal(err)
	}
	mutationRoot, err := db.CreateInvestigation(ctx, model.InvestigationNew{ProjectID: project, Title: "runtime mutation case"})
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
	childSelection := model.GatewaySelection{Events: []model.GatewayEvent{{SnapshotID: "child", Direct: true, Title: "child event", EventType: "network", OccurredAt: at, Provenance: model.GatewayProvenance{Source: "pt-nad", ExternalID: "runtime:child", FetchedAt: at}}}}
	if _, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: child.ID, Origin: "analyst", Selection: childSelection}); err != nil {
		t.Fatal(err)
	}
	sourceSelection := func(device, instance, ip string) model.GatewaySelection {
		provenance := func(id string) model.GatewayProvenance {
			return model.GatewayProvenance{Source: "pt-nad", ExternalID: id, FetchedAt: at}
		}
		return model.GatewaySelection{
			Entities: []model.GatewayEntity{
				{SnapshotID: "device", TypeCode: "device", Value: "pt-nad:" + device, Attributes: map[string]any{"identity_method": "pt-nad-host-id", "source_instance": instance}, Provenance: []model.GatewayProvenance{provenance("device:" + device)}},
				{SnapshotID: "ip", TypeCode: "ip", Value: ip, Provenance: []model.GatewayProvenance{provenance("ip:" + ip)}},
			},
			Events: []model.GatewayEvent{
				{SnapshotID: "parent", Direct: true, Title: "session " + device, EventType: "network_session", OccurredAt: at, Provenance: provenance("parent:" + device), Entities: []model.GatewayEventEntity{{SnapshotID: "device", Roles: []string{"mentions"}}, {SnapshotID: "ip", Roles: []string{"mentions"}}}},
				{SnapshotID: "part", Direct: true, Title: "part " + device, EventType: "file", OccurredAt: at, Provenance: provenance("part:" + device), Attributes: map[string]any{"relation_type": "subevent_of", "parent_source_event_id": "parent:" + device}},
			},
			Relations: []model.GatewayRelation{{SnapshotID: "device-ip", RelationCode: "has_identifier", SourceEntitySnapshotID: "device", TargetEntitySnapshotID: "ip", OccurredAt: &at, Provenance: provenance("identifier:" + device)}},
		}
	}
	type familyIDs struct{ entity, event string }
	importedFamilies := make([]familyIDs, 0, 2)
	for _, fixture := range []struct{ device, instance, ip string }{{"one", "19", "10.0.0.1"}, {"two", "20", "10.0.0.2"}} {
		stats, err := db.ImportContext(ctx, model.ImportRequest{ProjectID: project, InvestigationID: mutationRoot.ID, Origin: "analyst", Selection: sourceSelection(fixture.device, fixture.instance, fixture.ip)})
		if err != nil {
			t.Fatal(err)
		}
		ids := familyIDs{}
		for _, group := range stats.Groups {
			switch group.Family {
			case "entity":
				ids.entity = group.GroupID
			case "event":
				ids.event = group.GroupID
			}
		}
		if ids.entity == "" || ids.event == "" {
			t.Fatalf("source import has incomplete grouping: %+v", stats.Groups)
		}
		importedFamilies = append(importedFamilies, ids)
	}
	nodes, err := db.GraphNodes(ctx, project, inv.ID, model.NodeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	hypothesis, err := db.CreateHypothesis(ctx, model.HypothesisNew{ProjectID: project, InvestigationID: inv.ID, Statement: "runtime hypothesis"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddHypothesisNode(ctx, project, inv.ID, hypothesis.ID, nodes[0].ID); err != nil {
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
	mutationBase := "/api/v1/investigations/" + mutationRoot.ID
	readGroup := func(family, id string) model.Group {
		var group model.Group
		path := mutationBase + "/" + family + "-groups/" + id
		if err := json.Unmarshal(call("GET", path, nil, project, true, 200), &group); err != nil {
			t.Fatal(err)
		}
		return group
	}
	mutateGroup := func(family, id, action string, body any) []model.Group {
		var result struct {
			Groups []model.Group `json:"groups"`
		}
		path := mutationBase + "/" + family + "-groups/" + id + "/" + action
		raw := call("POST", path, body, project, true, 200)
		if err := json.Unmarshal(raw, &result); err != nil || len(result.Groups) == 0 {
			t.Fatalf("%s %s response: %s %v", family, action, raw, err)
		}
		return result.Groups
	}
	entityA := readGroup("entity", importedFamilies[0].entity)
	entityB := readGroup("entity", importedFamilies[1].entity)
	entityReview := model.GroupReview{OperationID: uuid.NewString(), Version: entityA.Version, Reason: "runtime entity review", Members: []model.GroupReviewMember{{ID: entityA.Members[0].ID, Version: entityA.Members[0].Version, Status: "confirmed"}}}
	entityA = mutateGroup("entity", entityA.ID, "review", entityReview)[0]
	entityHistory := call("GET", mutationBase+"/entity-groups/"+entityA.ID+"/history?limit=10", nil, project, true, 200)
	if !bytes.Contains(entityHistory, []byte("test-reviewer")) {
		t.Fatal("entity history has no authenticated actor")
	}
	exerciseMergeSplit := func(family string, target, source model.Group) {
		merge := model.GroupMerge{OperationID: uuid.NewString(), Version: target.Version, Reason: "runtime merge", Sources: []model.GroupVersion{{ID: source.ID, Version: source.Version}}}
		for _, member := range target.Members {
			merge.Members = append(merge.Members, model.GroupPlacement{MemberID: member.ID, Role: member.Role, Ordinal: member.Ordinal})
		}
		for _, member := range source.Members {
			role := member.Role
			if family == "event" {
				role = "part"
			}
			merge.Members = append(merge.Members, model.GroupPlacement{MemberID: member.ID, Role: role, Ordinal: member.Ordinal})
		}
		mergedResult := mutateGroup(family, target.ID, "merge", merge)
		if len(mergedResult) != 2 || mergedResult[0].State != "active" || mergedResult[1].State != "superseded" {
			t.Fatalf("%s merge lineage: %+v", family, mergedResult)
		}
		merged := mergedResult[0]
		sourceRoles := map[string]string{}
		for _, member := range source.Members {
			sourceRoles[member.ObjectID] = member.Role
		}
		split := model.GroupSplit{OperationID: uuid.NewString(), Version: merged.Version, Reason: "runtime split", Partitions: []model.GroupPartition{{Title: "runtime first"}, {Title: "runtime second"}}}
		for _, member := range merged.Members {
			partition, role := 0, member.Role
			if originalRole, ok := sourceRoles[member.ObjectID]; ok {
				partition, role = 1, originalRole
			}
			split.Partitions[partition].Members = append(split.Partitions[partition].Members, model.GroupPlacement{MemberID: member.ID, Role: role, Ordinal: member.Ordinal})
		}
		splitResult := mutateGroup(family, merged.ID, "split", split)
		if len(splitResult) != 3 || splitResult[0].State != "superseded" || splitResult[1].State != "active" || splitResult[2].State != "active" {
			t.Fatalf("%s split lineage: %+v", family, splitResult)
		}
	}
	exerciseMergeSplit("entity", entityA, entityB)
	exerciseMergeSplit("event", readGroup("event", importedFamilies[0].event), readGroup("event", importedFamilies[1].event))
	base := "/api/v1/investigations/" + inv.ID
	call("GET", base+"/graph/projection", nil, project, false, 401)
	call("GET", base+"/graph/projection", nil, "ffffffffffff", true, 404)
	call("GET", base+"/graph/projection?min_confidence=2", nil, project, true, 422)
	var subtree model.GraphProjection
	if err := json.Unmarshal(call("GET", base+"/graph/projection?include_subtree=true&statuses=confirmed&min_confidence=0", nil, project, true, 200), &subtree); err != nil || !subtree.IncludeSubtree || len(subtree.RawNodes) != 3 {
		t.Fatalf("HTTP subtree projection: %+v %v", subtree, err)
	}
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
	var hypothesisProjection model.GraphProjection
	hypothesisPath := base + "/hypotheses/" + hypothesis.ID + "/graph/projection?statuses=confirmed&min_confidence=0"
	if err := json.Unmarshal(call("GET", hypothesisPath, nil, project, true, 200), &hypothesisProjection); err != nil || hypothesisProjection.HypothesisID == nil || *hypothesisProjection.HypothesisID != hypothesis.ID || len(hypothesisProjection.RawNodes) != 1 {
		t.Fatalf("HTTP hypothesis projection: %+v %v", hypothesisProjection, err)
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
