package httptransport

import (
	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func sourceToAPI(value domain.Source) api.Source {
	capabilities := make([]api.Capability, 0, len(value.Capabilities))
	for _, item := range value.Capabilities {
		capabilities = append(capabilities, api.Capability(item))
	}
	return api.Source{Code: value.Code, Name: value.Name, Kind: api.SourceKind(value.Kind), Mode: api.SourceMode(value.Mode), Status: api.SourceStatus(value.Status), Capabilities: capabilities}
}

func provenanceToAPI(value domain.Provenance) api.Provenance {
	result := api.Provenance{Source: value.Source, ExternalId: value.ExternalID, FetchedAt: value.FetchedAt}
	if value.SourceURL != "" {
		result.SourceUrl = &value.SourceURL
	}
	return result
}

func entitiesToAPI(values []domain.Entity) []api.Entity {
	result := make([]api.Entity, 0, len(values))
	for _, value := range values {
		provenance := make([]api.Provenance, 0, len(value.Provenance))
		for _, item := range value.Provenance {
			provenance = append(provenance, provenanceToAPI(item))
		}
		result = append(result, api.Entity{Id: value.ID, Type: value.Type, Value: value.Value, Attributes: nonNilMap(value.Attributes), Provenance: provenance})
	}
	return result
}

func eventsToAPI(values []domain.Event) []api.Event {
	result := make([]api.Event, 0, len(values))
	for _, value := range values {
		result = append(result, api.Event{Id: value.ID, Type: value.Type, Title: value.Title, Severity: api.EventSeverity(value.Severity), OccurredAt: value.OccurredAt, EntityIds: value.EntityIDs, Attributes: nonNilMap(value.Attributes), Provenance: provenanceToAPI(value.Provenance)})
	}
	return result
}

func relationsToAPI(values []domain.Relation) []api.Relation {
	result := make([]api.Relation, 0, len(values))
	for _, value := range values {
		result = append(result, api.Relation{Id: value.ID, Type: value.Type, SourceEntityId: value.SourceEntityID, TargetEntityId: value.TargetEntityID, OccurredAt: value.OccurredAt, Provenance: provenanceToAPI(value.Provenance)})
	}
	return result
}

func artifactToAPI(value domain.Artifact) api.Artifact {
	result := api.Artifact{Id: value.ID, Name: value.Name, Hashes: hashesToAPI(value.Hashes)}
	if value.MIME != "" {
		result.Mime = &value.MIME
	}
	if value.Size > 0 {
		result.Size = &value.Size
	}
	return result
}

func analysisToAPI(value domain.Analysis) api.Analysis {
	artifacts := make([]api.Artifact, 0, len(value.Artifacts))
	for _, artifact := range value.Artifacts {
		artifacts = append(artifacts, artifactToAPI(artifact))
	}
	result := api.Analysis{Id: value.ID, Status: api.AnalysisStatus(value.Status), Artifact: artifactToAPI(value.Artifact), Verdict: verdictToAPI(value.Verdict), Artifacts: artifacts, Provenance: provenanceToAPI(value.Provenance)}
	if value.Attributes != nil {
		result.Attributes = &value.Attributes
	}
	return result
}

func endpointsToAPI(values []domain.Endpoint) []api.Endpoint {
	result := make([]api.Endpoint, 0, len(values))
	for _, value := range values {
		item := api.Endpoint{Id: value.ID, ExternalId: value.ExternalID, Hostname: value.Hostname, Status: api.EndpointStatus(value.Status), Attributes: nonNilMap(value.Attributes), Provenance: provenanceToAPI(value.Provenance)}
		if len(value.IPAddresses) > 0 {
			addresses := append([]string(nil), value.IPAddresses...)
			item.IpAddresses = &addresses
		}
		result = append(result, item)
	}
	return result
}

func sourceErrorsToAPI(values []domain.SourceError) []api.SourceError {
	result := make([]api.SourceError, 0, len(values))
	for _, value := range values {
		result = append(result, api.SourceError{Source: value.Source, Code: value.Code, Message: value.Message, Retryable: value.Retryable})
	}
	return result
}

func verdictsToAPI(values []domain.Verdict) []api.Verdict {
	result := make([]api.Verdict, 0, len(values))
	for _, value := range values {
		result = append(result, verdictToAPI(value))
	}
	return result
}

func verdictToAPI(value domain.Verdict) api.Verdict {
	return api.Verdict{Value: api.VerdictValue(value.Value), Confidence: value.Confidence, Labels: nonNilSlice(value.Labels), Provider: value.Provider}
}

func hashesFromAPI(value api.Hashes) domain.Hashes {
	return domain.Hashes{MD5: stringValue(value.Md5), SHA1: stringValue(value.Sha1), SHA256: stringValue(value.Sha256)}
}

func hashesToAPI(value domain.Hashes) api.Hashes {
	return api.Hashes{Md5: stringPointer(value.MD5), Sha1: stringPointer(value.SHA1), Sha256: stringPointer(value.SHA256)}
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}
