package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/mockdata"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const SourceCode = "pt-sandbox"

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
	return analysisFor(artifact), nil
}

func (adapter *mock) GetAnalysis(ctx context.Context, analysisID string) (domain.Analysis, error) {
	if err := ctx.Err(); err != nil {
		return domain.Analysis{}, err
	}
	for _, name := range mockdata.KnownArtifactNames {
		analysis := analysisFor(mockdata.Artifact(name))
		if analysis.ID.String() == analysisID {
			return analysis, nil
		}
	}
	return domain.Analysis{}, fmt.Errorf("%w: analysis %q", domain.ErrNotFound, analysisID)
}

func analysisFor(artifact domain.Artifact) domain.Analysis {
	extracted := []domain.Artifact{}
	switch artifact.Name {
	case "malicious_office_document.docx", "veeam_1272":
		extracted = append(extracted, mockdata.Artifact("pilot.ps1"))
	case "shell.php":
		extracted = append(extracted, mockdata.Artifact("transfer.php"))
	}
	labels := []string{"behavioral-analysis", "incident-scenario"}
	if strings.HasSuffix(artifact.Name, ".docx") {
		labels = append(labels, "office-macro")
	}
	return domain.Analysis{
		ID:       domain.StableID("analysis", SourceCode, artifact.Hashes.SHA256),
		Status:   "completed",
		Artifact: artifact,
		Verdict: domain.Verdict{
			Value:      "malicious",
			Confidence: 0.99,
			Labels:     labels,
			Provider:   SourceCode,
		},
		Artifacts: extracted,
		Attributes: map[string]any{
			"scan_state": "FULL",
			"engines": []map[string]any{
				{"code": "ptesc", "subsystem": "STATIC", "verdict": "DANGEROUS"},
				{"code": "sandbox", "subsystem": "BEHAVIOR", "verdict": "DANGEROUS"},
			},
			"behaviors": []string{"downloads payload", "creates process", "contacts staging host"},
		},
		Provenance: domain.Provenance{
			Source:     SourceCode,
			ExternalID: "scan-" + artifact.Hashes.SHA256[:12],
			FetchedAt:  fetchedAt,
		},
	}
}
