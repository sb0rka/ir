package capability

import (
	"context"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type SearchEventsRequest struct {
	TimeFrom time.Time
	TimeTo   time.Time
	Query    string
	Entities []domain.EntityRef
	Limit    int
	Cursor   string
}

type EventPage struct {
	Events        []domain.Event
	Entities      []domain.Entity
	Relations     []domain.Relation
	Continuations []string
	HasMore       bool
}

type EventSource interface {
	SearchEvents(context.Context, SearchEventsRequest) (EventPage, error)
}

type LookupEntityRequest struct {
	Entity domain.EntityRef
}

type LookupEntityResult struct {
	Entities  []domain.Entity
	Relations []domain.Relation
	Verdicts  []domain.Verdict
}

type EntityLookup interface {
	LookupEntity(context.Context, LookupEntityRequest) (LookupEntityResult, error)
}

type AnalyzeArtifactRequest struct {
	Name   string
	MIME   string
	Hashes domain.Hashes
}

type ArtifactAnalyzer interface {
	AnalyzeArtifact(context.Context, AnalyzeArtifactRequest) (domain.Analysis, error)
	GetAnalysis(context.Context, string) (domain.Analysis, error)
}

type SearchEndpointsRequest struct {
	Query  string
	Limit  int
	Cursor string
}

type EndpointPage struct {
	Items         []domain.Endpoint
	Continuations []string
	HasMore       bool
}

type EndpointSource interface {
	SearchEndpoints(context.Context, SearchEndpointsRequest) (EndpointPage, error)
}

type ResponseCatalog interface {
	ListResponseActions(context.Context, string) ([]domain.ResponseAction, error)
}
