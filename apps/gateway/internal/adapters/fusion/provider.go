package fusion

import (
	"context"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mockdata"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const SourceCode = "pt-fusion"

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type mock struct{}

func NewMock() registry.Provider {
	adapter := &mock{}
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "PT Fusion",
			Kind:         "threat_intelligence",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEntityLookup},
		},
		EntityLookup: adapter,
	}
}

func (adapter *mock) LookupEntity(ctx context.Context, request capability.LookupEntityRequest) (capability.LookupEntityResult, error) {
	if err := ctx.Err(); err != nil {
		return capability.LookupEntityResult{}, err
	}
	provenance := domain.Provenance{
		Source:     SourceCode,
		ExternalID: strings.ToLower(request.Entity.Type) + ":" + domain.CanonicalValue(request.Entity.Type, request.Entity.Value),
		FetchedAt:  fetchedAt,
	}
	entity := domain.NewEntity(request.Entity.Type, request.Entity.Value, provenance)
	malicious, artifactName := isMalicious(entity)
	verdict := domain.Verdict{Value: "unknown", Confidence: 0.1, Labels: []string{}, Provider: SourceCode}
	entity.Attributes["reputation"] = "unknown"
	entity.Attributes["potential_damage"] = "unknown"
	if malicious {
		verdict.Value = "malicious"
		verdict.Confidence = 0.97
		verdict.Labels = []string{"staging-infrastructure", "incident-scenario"}
		entity.Attributes["reputation"] = "malicious"
		entity.Attributes["potential_damage"] = "high"
	}
	result := capability.LookupEntityResult{Entities: []domain.Entity{entity}, Verdicts: []domain.Verdict{verdict}}

	if entity.Type == "ip" && entity.Value == "10.125.11.90" {
		domainEntity := domain.NewEntity("domain", "stager.qa-ptlab.ru", provenance)
		result.Entities = append(result.Entities, domainEntity)
		result.Relations = append(result.Relations, relation(entity, domainEntity, "resolves_to", provenance))
	}
	if artifactName != "" {
		file := domain.NewEntity("file", artifactName, provenance)
		result.Entities = append(result.Entities, file)
		result.Relations = append(result.Relations, relation(file, entity, "has_hash", provenance))
	}
	return result, nil
}

func isMalicious(entity domain.Entity) (bool, string) {
	if entity.Type == "ip" && entity.Value == "10.125.11.90" ||
		entity.Type == "domain" && entity.Value == "stager.qa-ptlab.ru" {
		return true, ""
	}
	if entity.Type != "hash" && entity.Type != "file_hash" {
		return false, ""
	}
	for _, name := range mockdata.KnownArtifactNames {
		artifact := mockdata.Artifact(name)
		if strings.EqualFold(entity.Value, artifact.Hashes.MD5) ||
			strings.EqualFold(entity.Value, artifact.Hashes.SHA1) ||
			strings.EqualFold(entity.Value, artifact.Hashes.SHA256) {
			return true, name
		}
	}
	return false, ""
}

func relation(source, target domain.Entity, kind string, provenance domain.Provenance) domain.Relation {
	return domain.Relation{
		ID:             domain.StableID("relation", SourceCode, kind, source.ID.String(), target.ID.String()),
		Type:           kind,
		SourceEntityID: source.ID,
		TargetEntityID: target.ID,
		Provenance:     provenance,
	}
}
