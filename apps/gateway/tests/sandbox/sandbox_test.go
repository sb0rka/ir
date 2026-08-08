package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/mockdata"
	mocksandbox "github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/sandbox"
	sandboxapi "github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/sandbox"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

var testFetchedAt = time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)

func TestMockRoundTripsThroughMapper(t *testing.T) {
	provider := mocksandbox.NewMock()
	artifact := mockdata.Artifact("malicious_office_document.docx")
	analysis, err := provider.ArtifactAnalyzer.AnalyzeArtifact(context.Background(), capability.AnalyzeArtifactRequest{
		Name:   artifact.Name,
		MIME:   artifact.MIME,
		Hashes: artifact.Hashes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Provenance.Source != mocksandbox.SourceCode || analysis.Verdict.Value != "malicious" || len(analysis.Artifacts) != 1 {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
	stored, err := provider.ArtifactAnalyzer.GetAnalysis(context.Background(), analysis.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(analysis)
	right, _ := json.Marshal(stored)
	if !bytes.Equal(left, right) {
		t.Fatal("stored vendor response does not map byte-for-byte identically")
	}
}

func TestContractFixturesAndMapper(t *testing.T) {
	requestRaw := readFixture(t, "testdata/request.json")
	var request sandboxapi.CreateScanTaskRequest
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	if request.FileURI == "" || request.Options == nil || request.Options.Sandbox == nil {
		t.Fatalf("unexpected request DTO: %#v", request)
	}

	responseRaw := readFixture(t, "testdata/response.json")
	var response sandboxapi.Response[sandboxapi.ScanData]
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	analysis, err := sandboxapi.ToAnalysis(response, testFetchedAt)
	if err != nil {
		t.Fatalf("map response: %v", err)
	}
	if analysis.Status != "completed" || analysis.Verdict.Value != "malicious" || analysis.Artifact.Name != "sample.txt" {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
	if analysis.Provenance.ExternalID != response.Data.ScanID || len(analysis.Verdict.Labels) != 2 {
		t.Fatalf("provenance or labels were not mapped: %#v", analysis)
	}
	first, err := json.Marshal(analysis)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(analysis)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("mapper output is not byte-stable")
	}
}

func TestToAnalysisRejectsMalformedResponse(t *testing.T) {
	_, err := sandboxapi.ToAnalysis(sandboxapi.Response[sandboxapi.ScanData]{Data: sandboxapi.ScanData{ScanID: "scan-id"}}, testFetchedAt)
	if err == nil {
		t.Fatal("expected missing primary artifact to fail")
	}
	_, err = sandboxapi.ToAnalysis(sandboxapi.Response[sandboxapi.ScanData]{Errors: []sandboxapi.APIError{{Type: "HTTPUnauthorized", Message: "Authorization required"}}}, testFetchedAt)
	if err == nil {
		t.Fatal("expected vendor error to fail")
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
