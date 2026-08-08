package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/mockdata"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

func TestMockRoundTripsThroughMapper(t *testing.T) {
	provider := NewMock()
	artifact := mockdata.Artifact("malicious_office_document.docx")
	vendorRequest := toCreateScanTaskRequest(artifact)
	if vendorRequest.FileName != artifact.Name || vendorRequest.ShortResult == nil || *vendorRequest.ShortResult {
		t.Fatalf("unexpected vendor request: %#v", vendorRequest)
	}
	analysis, err := provider.ArtifactAnalyzer.AnalyzeArtifact(context.Background(), capability.AnalyzeArtifactRequest{
		Name:   artifact.Name,
		MIME:   artifact.MIME,
		Hashes: artifact.Hashes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Provenance.Source != SourceCode || analysis.Verdict.Value != "malicious" || len(analysis.Artifacts) != 1 {
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
