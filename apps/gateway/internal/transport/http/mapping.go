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
		sources := make([]api.EntitySource, 0, len(value.Provenance))
		for _, item := range value.Provenance {
			source := api.EntitySource{SourceCode: item.Source, SourceEntityId: item.ExternalID, FetchedAt: item.FetchedAt}
			if item.SourceURL != "" {
				source.SourceRef = &item.SourceURL
			}
			sources = append(sources, source)
		}
		result = append(result, api.Entity{Type: value.Type, Value: value.Value, Attributes: nonNilMap(value.Attributes), Sources: sources})
	}
	return result
}

func eventsToAPI(values []domain.Event) []api.Event {
	result := make([]api.Event, 0, len(values))
	for _, value := range values {
		item := api.Event{
			SourceCode: value.Provenance.Source, SourceEventId: value.Provenance.ExternalID,
			Type: value.Type, Title: value.Title, Severity: api.EventSeverity(value.Severity),
			OccurredAt: value.OccurredAt, Entities: entityMentionsToAPI(value.Entities), Attributes: nonNilMap(value.Attributes), FetchedAt: value.Provenance.FetchedAt,
		}
		if value.Provenance.SourceURL != "" {
			item.SourceRef = &value.Provenance.SourceURL
		}
		result = append(result, item)
	}
	return result
}

func findingsToAPI(values []domain.Finding) []api.Finding {
	result := make([]api.Finding, 0, len(values))
	for _, value := range values {
		item := api.Finding{
			Ref: sourceObjectRefToAPI(value.Ref), Kind: api.FindingKind(value.Kind), Title: value.Title,
			Severity: api.FindingSeverity(value.Severity), OccurredAt: value.OccurredAt,
			Entities: entityMentionsToAPI(value.Entities), FetchedAt: value.FetchedAt,
		}
		item.Description = stringPointer(value.Description)
		item.Status = stringPointer(value.Status)
		item.SourceRef = stringPointer(value.SourceRef)
		if len(value.RelatedFindings) > 0 {
			refs := sourceObjectRefsToAPI(value.RelatedFindings)
			item.RelatedFindings = &refs
		}
		if len(value.RelatedSessions) > 0 {
			refs := sourceObjectRefsToAPI(value.RelatedSessions)
			item.RelatedSessions = &refs
		}
		if value.Rule != nil {
			item.Rule = &api.RuleRef{Id: stringPointer(value.Rule.ID), Name: value.Rule.Name}
		}
		if value.Incident != nil {
			item.Incident = &api.IncidentDetails{
				Key: stringPointer(value.Incident.Key), ExternalKey: stringPointer(value.Incident.ExternalKey),
				Type: stringPointer(value.Incident.Type),
				Verdict: stringPointer(value.Incident.Verdict), Damage: stringPointer(value.Incident.Damage),
				Recommendation: stringPointer(value.Incident.Recommendation), AssignedTo: stringPointer(value.Incident.AssignedTo),
				ChangedAt: value.Incident.ChangedAt, Archived: boolPointer(value.Incident.Archived), Removed: boolPointer(value.Incident.Removed),
			}
		}
		if value.Correlation != nil {
			item.Correlation = &api.CorrelationDetails{
				CorrelationType: stringPointer(value.Correlation.CorrelationType), SubeventCount: intPointer(value.Correlation.SubeventCount),
			}
		}
		if value.NADAttack != nil {
			item.NadAttack = &api.NADAttackDetails{
				Class: stringPointer(value.NADAttack.Class), Gid: intPointer(value.NADAttack.GID), Sid: intPointer(value.NADAttack.SID),
				Revision: intPointer(value.NADAttack.Revision), RawPriority: intPointer(value.NADAttack.RawPriority), FalsePositive: value.NADAttack.FalsePositive,
			}
		}
		result = append(result, item)
	}
	return result
}

func sessionsToAPI(values []domain.Session) []api.Session {
	result := make([]api.Session, 0, len(values))
	for _, value := range values {
		item := api.Session{
			Ref: sourceObjectRefToAPI(value.Ref), Title: value.Title, Severity: api.SessionSeverity(value.Severity),
			AuthenticationHints: authenticationHintsToAPI(value.AuthenticationHints), FileHints: fileHintsToAPI(value.FileHints),
			RawCriticality: value.RawCriticality, StartedAt: value.StartedAt, EndedAt: value.EndedAt,
			DurationSeconds: value.DurationSeconds, SourceEndpoint: networkEndpointToAPI(value.SourceEndpoint),
			DestinationEndpoint: networkEndpointToAPI(value.DestinationEndpoint), TransportProtocol: value.TransportProtocol,
			ApplicationProtocol: stringPointer(value.ApplicationProtocol), State: nonNilSlice(value.State),
			FalsePositive: value.FalsePositive, HasFiles: value.HasFiles, Entities: entityMentionsToAPI(value.Entities),
			RelatedFindings: sourceObjectRefsToAPI(value.RelatedFindings), SourceRef: stringPointer(value.SourceRef), FetchedAt: value.FetchedAt,
		}
		if value.Bytes != nil {
			item.Bytes = trafficCountersToAPI(*value.Bytes)
		}
		if value.Packets != nil {
			item.Packets = trafficCountersToAPI(*value.Packets)
		}
		if len(value.TCPFlags) > 0 {
			flags := append([]string(nil), value.TCPFlags...)
			item.TcpFlags = &flags
		}
		result = append(result, item)
	}
	return result
}

func fileHintsToAPI(values []domain.SessionFileHint) []api.SessionFileHint {
	result := make([]api.SessionFileHint, 0, len(values))
	for _, value := range values {
		result = append(result, api.SessionFileHint{
			ExternalId: value.ExternalID, Name: stringPointer(value.Name), Mime: stringPointer(value.MIME), Size: value.Size,
			Md5: stringPointer(value.MD5), Sha256: stringPointer(value.SHA256), State: stringPointer(value.State), Direction: stringPointer(value.Direction),
		})
	}
	return result
}

func authenticationHintsToAPI(values []domain.SessionAuthenticationHint) []api.SessionAuthenticationHint {
	result := make([]api.SessionAuthenticationHint, 0, len(values))
	for _, value := range values {
		result = append(result, api.SessionAuthenticationHint{
			Protocol: value.Protocol, Method: stringPointer(value.Method), Account: stringPointer(value.Account), Valid: value.Valid,
			FailedAttempts: value.FailedAttempts, ClientHost: stringPointer(value.ClientHost), ServerHost: stringPointer(value.ServerHost),
		})
	}
	return result
}

func sourceObjectRefToAPI(value domain.SourceObjectRef) api.SourceObjectRef {
	return api.SourceObjectRef{
		SourceCode: value.SourceCode, SourceInstance: stringPointer(value.SourceInstance),
		RecordType: api.SourceObjectRefRecordType(value.RecordType), ExternalId: value.ExternalID,
		TimeRange: api.TimeRange{From: value.TimeRange.From, To: value.TimeRange.To},
	}
}

func sourceObjectRefsToAPI(values []domain.SourceObjectRef) []api.SourceObjectRef {
	result := make([]api.SourceObjectRef, 0, len(values))
	for _, value := range values {
		result = append(result, sourceObjectRefToAPI(value))
	}
	return result
}

func entityMentionsToAPI(values []domain.EntityMention) []api.EntityMention {
	result := make([]api.EntityMention, 0, len(values))
	for _, value := range values {
		roles := make([]api.EntityMentionRoles, 0, len(value.Roles))
		for _, role := range value.Roles {
			roles = append(roles, api.EntityMentionRoles(role))
		}
		result = append(result, api.EntityMention{Type: value.Type, Value: value.Value, Roles: roles})
	}
	return result
}

func sourceStatesToAPI(values []domain.SourceState) []api.SourceState {
	result := make([]api.SourceState, 0, len(values))
	for _, value := range values {
		result = append(result, api.SourceState{Source: value.Source, Status: api.SourceStateStatus(value.Status)})
	}
	return result
}

func resolutionsToAPI(values []domain.ObjectResolution) []api.ObjectResolution {
	result := make([]api.ObjectResolution, 0, len(values))
	for _, value := range values {
		result = append(result, api.ObjectResolution{
			Ref: sourceObjectRefToAPI(value.Ref), Status: api.ObjectResolutionStatus(value.Status), Errors: sourceErrorsToAPI(value.Errors),
		})
	}
	return result
}

func networkEndpointToAPI(value domain.NetworkEndpoint) api.NetworkEndpoint {
	return api.NetworkEndpoint{Ip: stringPointer(value.IP), Mac: stringPointer(value.MAC), Host: stringPointer(value.Host), Port: value.Port}
}

func trafficCountersToAPI(value domain.TrafficCounters) *api.TrafficCounters {
	return &api.TrafficCounters{Sent: int64Pointer(value.Sent), Received: int64Pointer(value.Received), Total: int64Pointer(value.Total)}
}

func relationsToAPI(values []domain.Relation) []api.Relation {
	result := make([]api.Relation, 0, len(values))
	for _, value := range values {
		item := api.Relation{
			Type:         value.Type,
			SourceEntity: api.EntityRef{Type: value.SourceEntity.Type, Value: value.SourceEntity.Value},
			TargetEntity: api.EntityRef{Type: value.TargetEntity.Type, Value: value.TargetEntity.Value},
			OccurredAt:   value.OccurredAt, SourceCode: value.Provenance.Source,
			SourceRelationId: value.Provenance.ExternalID, FetchedAt: value.Provenance.FetchedAt,
		}
		if value.Provenance.SourceURL != "" {
			item.SourceRef = &value.Provenance.SourceURL
		}
		result = append(result, item)
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

func intPointer(value int) *int { return &value }

func int64Pointer(value int64) *int64 { return &value }

func boolPointer(value bool) *bool { return &value }

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
