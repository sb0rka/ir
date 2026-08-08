package sandbox

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const SourceCode = "pt-sandbox"

func ToAnalysis(response Response[ScanData], fetchedAt time.Time) (domain.Analysis, error) {
	if len(response.Errors) > 0 {
		return domain.Analysis{}, fmt.Errorf("PT Sandbox error %q: %s", response.Errors[0].Type, response.Errors[0].Message)
	}
	if strings.TrimSpace(response.Data.ScanID) == "" {
		return domain.Analysis{}, fmt.Errorf("PT Sandbox scan_id is required")
	}
	if len(response.Data.Artifacts) == 0 {
		return domain.Analysis{}, fmt.Errorf("PT Sandbox response has no primary artifact")
	}
	primary, err := toArtifact(response.Data.Artifacts[0])
	if err != nil {
		return domain.Analysis{}, fmt.Errorf("map primary artifact: %w", err)
	}
	if !strings.EqualFold(response.Data.Result.ScanState, "FULL") {
		return domain.Analysis{}, fmt.Errorf("PT Sandbox scan is not complete: %q", response.Data.Result.ScanState)
	}

	extracted := make([]domain.Artifact, 0)
	for index, artifact := range response.Data.Artifacts[0].Artifacts {
		item, err := toArtifact(artifact)
		if err != nil {
			return domain.Analysis{}, fmt.Errorf("map extracted artifact %d: %w", index, err)
		}
		extracted = append(extracted, item)
	}
	sort.Slice(extracted, func(i, j int) bool { return extracted[i].ID.String() < extracted[j].ID.String() })

	labels := make([]string, 0)
	engines := make([]map[string]any, 0, len(response.Data.Artifacts[0].EngineResults))
	for _, engine := range response.Data.Artifacts[0].EngineResults {
		engines = append(engines, map[string]any{
			"code":      engine.EngineCodeName,
			"subsystem": engine.EngineSubsystem,
			"verdict":   engine.Result.Verdict,
		})
		for _, detection := range engine.Detections {
			if value := strings.ToLower(strings.TrimSpace(detection.Threat)); value != "" {
				labels = append(labels, value)
			}
			if value := strings.ToLower(strings.TrimSpace(detection.Detect)); value != "" {
				labels = append(labels, value)
			}
		}
	}
	labels = sortedUnique(labels)

	return domain.Analysis{
		ID:        domain.StableID("analysis", SourceCode, response.Data.ScanID),
		Status:    "completed",
		Artifact:  primary,
		Artifacts: extracted,
		Verdict: domain.Verdict{
			Value:      verdict(response.Data.Result.Verdict),
			Confidence: 1,
			Labels:     labels,
			Provider:   SourceCode,
		},
		Attributes: map[string]any{
			"duration":   response.Data.Result.Duration,
			"engines":    engines,
			"scan_state": response.Data.Result.ScanState,
			"threat":     response.Data.Result.Threat,
		},
		Provenance: domain.Provenance{
			Source:     SourceCode,
			ExternalID: response.Data.ScanID,
			FetchedAt:  fetchedAt,
		},
	}, nil
}

func toArtifact(value Artifact) (domain.Artifact, error) {
	info := value.FileInfo
	if strings.TrimSpace(info.SHA256) == "" {
		return domain.Artifact{}, fmt.Errorf("file_info.sha256 is required")
	}
	name := strings.TrimSpace(info.FilePath)
	if name == "" {
		name = info.SHA256
	}
	if info.Size > math.MaxInt64 {
		return domain.Artifact{}, fmt.Errorf("file_info.size exceeds int64")
	}
	return domain.Artifact{
		ID:   domain.StableID("artifact", info.SHA256),
		Name: name,
		MIME: info.MIMEType,
		Size: int64(info.Size),
		Hashes: domain.Hashes{
			MD5:    info.MD5,
			SHA1:   info.SHA1,
			SHA256: info.SHA256,
		},
	}, nil
}

func verdict(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DANGEROUS":
		return "malicious"
	case "SAFE":
		return "benign"
	case "SUSPICIOUS":
		return "suspicious"
	default:
		return "unknown"
	}
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
