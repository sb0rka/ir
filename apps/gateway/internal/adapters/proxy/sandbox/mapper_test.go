package sandbox

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

var testFetchedAt = time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)

func TestContractFixturesAndMapper(t *testing.T) {
	requestRaw := readFixture(t, "testdata/request.json")
	var request CreateScanTaskRequest
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	if request.FileURI == "" || request.Options == nil || request.Options.Sandbox == nil {
		t.Fatalf("unexpected request DTO: %#v", request)
	}

	responseRaw := readFixture(t, "testdata/response.json")
	var response Response[ScanData]
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	analysis, err := ToAnalysis(response, testFetchedAt)
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
	_, err := ToAnalysis(Response[ScanData]{Data: ScanData{ScanID: "scan-id"}}, testFetchedAt)
	if err == nil {
		t.Fatal("expected missing primary artifact to fail")
	}
	_, err = ToAnalysis(Response[ScanData]{Errors: []APIError{{Type: "HTTPUnauthorized", Message: "Authorization required"}}}, testFetchedAt)
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
