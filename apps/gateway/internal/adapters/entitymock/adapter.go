package entitymock

import (
	"context"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

var fetchedAt = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type VerdictFunc func(domain.Entity) []domain.Verdict

type Adapter struct {
	scenario   scenario.Scenario
	system     string
	sourceCode string
	verdicts   VerdictFunc
}

func New(value scenario.Scenario, system, sourceCode string, verdicts VerdictFunc) *Adapter {
	return &Adapter{scenario: value, system: system, sourceCode: sourceCode, verdicts: verdicts}
}

func (adapter *Adapter) LookupEntity(ctx context.Context, request capability.LookupEntityRequest) (capability.LookupEntityResult, error) {
	if err := ctx.Err(); err != nil {
		return capability.LookupEntityResult{}, err
	}
	wantedType := strings.ToLower(strings.TrimSpace(request.Entity.Type))
	wantedValue := domain.CanonicalValue(wantedType, request.Entity.Value)
	nodeEntities := make(map[string]domain.Entity)
	matched := make(map[string]domain.Entity)

	for _, node := range adapter.scenario.NodesForSystem(adapter.system) {
		entity, ok := adapter.scenario.EntityForNode(node, adapter.sourceCode, fetchedAt)
		if !ok {
			continue
		}
		nodeEntities[node.ID] = entity
		if strings.EqualFold(entity.Type, wantedType) && entity.Value == wantedValue {
			matched[node.ID] = entity
		}
	}
	if len(matched) == 0 {
		return capability.LookupEntityResult{}, nil
	}

	result := capability.LookupEntityResult{}
	for nodeID, entity := range matched {
		result.Entities = append(result.Entities, entity)
		for _, edge := range adapter.scenario.Edges {
			if edge.Source != nodeID && edge.Target != nodeID {
				continue
			}
			otherID := edge.Target
			if otherID == nodeID {
				otherID = edge.Source
			}
			other, ok := nodeEntities[otherID]
			if !ok {
				continue
			}
			source, sourceOK := nodeEntities[edge.Source]
			target, targetOK := nodeEntities[edge.Target]
			if !sourceOK || !targetOK {
				continue
			}
			result.Entities = append(result.Entities, other)
			provenance := domain.Provenance{Source: adapter.sourceCode, ExternalID: edge.ID, FetchedAt: fetchedAt}
			result.Relations = append(result.Relations, domain.Relation{
				ID:             domain.StableID("relation", adapter.sourceCode, edge.ID),
				Type:           normalize(edge.Label),
				SourceEntityID: source.ID,
				TargetEntityID: target.ID,
				Provenance:     provenance,
			})
		}
		if adapter.verdicts != nil {
			result.Verdicts = append(result.Verdicts, adapter.verdicts(entity)...)
		}
	}
	result.Entities = normalization.Entities(result.Entities)
	result.Relations = normalization.Relations(result.Relations)
	return result, nil
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(" ", "_", "-", "_").Replace(value)
}
