package capability

import (
	"context"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

// Access is constructed by the service from project-scoped Secrets. Public
// request bodies never provide credentials, vendor URLs, or source instances.
type Access struct {
	Cookie string
}

type SearchFindingsRequest struct {
	TimeRange domain.TimeRange
	Kinds     []string
	Limit     int
	Cursor    string
}

type FindingPage struct {
	Findings   []domain.Finding
	Status     string
	NextCursor string
}

type FindingSource interface {
	SearchFindings(context.Context, Access, SearchFindingsRequest) (FindingPage, error)
	ResolveFinding(context.Context, Access, domain.SourceObjectRef) (ContextPage, error)
}

type SearchSessionsRequest struct {
	TimeRange domain.TimeRange
	Limit     int
	Cursor    string
}

type SessionPage struct {
	Sessions   []domain.Session
	Status     string
	NextCursor string
}

type SessionSource interface {
	SearchSessions(context.Context, Access, SearchSessionsRequest) (SessionPage, error)
	ResolveSession(context.Context, Access, domain.SourceObjectRef) (ContextPage, error)
}

type SearchEventsRequest struct {
	TimeFrom time.Time
	TimeTo   time.Time
	Entities []domain.EntityRef
	Limit    int
	Cursor   string
}

type EventPage struct {
	Events     []domain.Event
	Entities   []domain.Entity
	Relations  []domain.Relation
	Status     string
	NextCursor string
}

type ResolveContextRequest struct {
	EventIDs  []string
	EntityIDs []string
}

type EventSource interface {
	SearchEvents(context.Context, Access, SearchEventsRequest) (EventPage, error)
	ResolveContext(context.Context, Access, ResolveContextRequest) (ContextPage, error)
}

type ContextPage struct {
	Findings    []domain.Finding
	Sessions    []domain.Session
	Events      []domain.Event
	Entities    []domain.Entity
	Relations   []domain.Relation
	Resolutions []domain.ObjectResolution
}

type LookupEntityRequest struct {
	Entity    domain.EntityRef
	TimeRange domain.TimeRange
}

type LookupEntityResult struct {
	Entities  []domain.Entity
	Relations []domain.Relation
	Verdicts  []domain.Verdict
}

type EntityLookup interface {
	LookupEntity(context.Context, Access, LookupEntityRequest) (LookupEntityResult, error)
}

type AnalyzeArtifactRequest struct {
	Name   string
	MIME   string
	Hashes domain.Hashes
}

type ArtifactAnalyzer interface {
	AnalyzeArtifact(context.Context, Access, AnalyzeArtifactRequest) (domain.Analysis, error)
	GetAnalysis(context.Context, Access, string) (domain.Analysis, error)
}

type SearchEndpointsRequest struct {
	Limit  int
	Cursor string
}

type EndpointPage struct {
	Items      []domain.Endpoint
	NextCursor string
}

type EndpointSource interface {
	SearchEndpoints(context.Context, Access, SearchEndpointsRequest) (EndpointPage, error)
}

type ResponseCatalog interface {
	ListResponseActions(context.Context, Access, string) ([]domain.ResponseAction, error)
}

type AccountUserinfoSource interface {
	GetAccountUserinfo(context.Context, Access) (domain.AccountUserinfo, error)
}

// SourceProber checks every backend represented by one logical provider. A
// successful probe may still be degraded when only some configured stores or
// SIEM backends respond.
type SourceProber interface {
	Probe(context.Context, Access) (string, error)
}
