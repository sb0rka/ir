package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/mockdata"
	sandboxapi "github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/sandbox"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const SourceCode = sandboxapi.SourceCode

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type mock struct{}

func NewMock() registry.Provider {
	adapter := &mock{}
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "PT Sandbox",
			Kind:         "sandbox",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityArtifactAnalysis},
		},
		ArtifactAnalyzer: adapter,
	}
}

func (adapter *mock) AnalyzeArtifact(ctx context.Context, request capability.AnalyzeArtifactRequest) (domain.Analysis, error) {
	if err := ctx.Err(); err != nil {
		return domain.Analysis{}, err
	}
	artifact, ok := mockdata.FindArtifact(request.Name, request.Hashes)
	if !ok {
		return domain.Analysis{}, &domain.RequestError{Code: "artifact_not_in_scenario", Message: "artifact is not present in the mock scenario"}
	}
	return sandboxapi.ToAnalysis(scanResponseFor(toCreateScanTaskRequest(artifact), artifact), fetchedAt)
}

func (adapter *mock) GetAnalysis(ctx context.Context, analysisID string) (domain.Analysis, error) {
	if err := ctx.Err(); err != nil {
		return domain.Analysis{}, err
	}
	for _, name := range mockdata.KnownArtifactNames {
		artifact := mockdata.Artifact(name)
		analysis, err := sandboxapi.ToAnalysis(scanResponseFor(toCreateScanTaskRequest(artifact), artifact), fetchedAt)
		if err != nil {
			return domain.Analysis{}, err
		}
		if analysis.ID.String() == analysisID {
			return analysis, nil
		}
	}
	return domain.Analysis{}, fmt.Errorf("%w: analysis %q", domain.ErrNotFound, analysisID)
}

func toCreateScanTaskRequest(artifact domain.Artifact) sandboxapi.CreateScanTaskRequest {
	shortResult := false
	return sandboxapi.CreateScanTaskRequest{
		FileURI:     "sfm-files:///mock/" + artifact.Hashes.SHA256,
		FileName:    artifact.Name,
		AsyncResult: false,
		ShortResult: &shortResult,
	}
}

func scanResponseFor(request sandboxapi.CreateScanTaskRequest, artifact domain.Artifact) sandboxapi.Response[sandboxapi.ScanData] {
	root := vendorArtifact(artifact)
	root.FileInfo.FilePath = request.FileName
	switch artifact.Name {
	case "malicious_office_document.docx", "veeam_1272":
		root.Artifacts = append(root.Artifacts, vendorArtifact(mockdata.Artifact("pilot.ps1")))
	case "shell.php":
		root.Artifacts = append(root.Artifacts, vendorArtifact(mockdata.Artifact("transfer.php")))
	}
	if strings.HasSuffix(artifact.Name, ".docx") {
		root.EngineResults[0].Detections = append(root.EngineResults[0].Detections, sandboxapi.Detection{Detect: "office-macro", Threat: "DOCUMENT"})
	}
	return sandboxapi.Response[sandboxapi.ScanData]{
		Data: sandboxapi.ScanData{
			ScanID:    "scan-" + artifact.Hashes.SHA256[:12],
			Artifacts: []sandboxapi.Artifact{root},
			Result: sandboxapi.ScanResult{
				Duration:  0.17754173884168268,
				ScanState: "FULL",
				Threat:    "MALWARE",
				Verdict:   "DANGEROUS",
			},
		},
		Errors: []sandboxapi.APIError{},
	}
}

func vendorArtifact(artifact domain.Artifact) sandboxapi.Artifact {
	result := sandboxapi.ScanResult{ScanState: "FULL", Threat: "MALWARE", Verdict: "DANGEROUS"}
	return sandboxapi.Artifact{
		Artifacts: []sandboxapi.Artifact{},
		EngineResults: []sandboxapi.EngineResult{
			{
				EngineCodeName:  "ptesc",
				EngineSubsystem: "STATIC",
				Detections:      []sandboxapi.Detection{{Detect: "incident-scenario", Threat: "MALWARE"}},
				Result:          result,
			},
			{EngineCodeName: "sandbox", EngineSubsystem: "SANDBOX", Result: result},
		},
		FileInfo: sandboxapi.FileInfo{
			FilePath: artifact.Name,
			FileURI:  "sha256:" + artifact.Hashes.SHA256,
			MD5:      artifact.Hashes.MD5,
			MIMEType: artifact.MIME,
			SHA1:     artifact.Hashes.SHA1,
			SHA256:   artifact.Hashes.SHA256,
			Size:     uint64(artifact.Size),
		},
		Result: result,
		Type:   "FILE",
	}
}
