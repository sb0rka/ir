// Package model contains transport-independent investigation domain values.
package model

import "time"

type SubjectRoles struct {
	ProjectID string
	Roles     []string
}

type PageCursor struct {
	Time time.Time
	ID   string
}

type InvestigationNew struct {
	ProjectID    string
	Title        string
	Description  *string
	Severity     *string
	ParentID     *string
	WorkspaceIDs []string
}

type Investigation struct {
	ID            string
	ProjectID     string
	ParentID      *string
	WorkspaceIDs  []string
	Title         string
	Description   *string
	Status        string
	Severity      *string
	Verdict       *string
	VerdictReason *string
	Confidence    *float32
	Origin        string
	OriginRef     *string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      *time.Time
	Counters      InvestigationCounters
}

type InvestigationCounters struct {
	Children      int
	Events        int
	Entities      int
	ProposedEdges int
}

type InvestigationFilter struct {
	ParentID  *string
	RootsOnly bool
	Status    *string
	Severity  *string
	Q         *string
	Cursor    *PageCursor
	Limit     int
}

type GatewayProvenance struct {
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	SourceURL  *string   `json:"source_url,omitempty"`
	FetchedAt  time.Time `json:"fetched_at"`
}

type GatewayEvent struct {
	SnapshotID        string
	Title             string
	EventType         string
	Severity          string
	OccurredAt        time.Time
	EntitySnapshotIDs []string
	Attributes        map[string]any
	Provenance        GatewayProvenance
}

type GatewayEntity struct {
	SnapshotID string
	TypeCode   string
	Value      string
	Attributes map[string]any
	Provenance []GatewayProvenance
}

type GatewayRelation struct {
	SnapshotID             string
	RelationCode           string
	SourceEntitySnapshotID string
	TargetEntitySnapshotID string
	OccurredAt             *time.Time
	Provenance             GatewayProvenance
}

type GatewaySelection struct {
	Events    []GatewayEvent
	Entities  []GatewayEntity
	Relations []GatewayRelation
}

type AgentNode struct {
	Ref              string
	SnapshotEventID  *string
	SnapshotEntityID *string
	NodeID           *string
}

type AgentEdge struct {
	SourceRef         string
	TargetRef         string
	RelationCode      string
	Why               string
	Confidence        *float32
	EvidenceEventRefs []string
}

type ImportRequest struct {
	ProjectID       string
	InvestigationID string
	Selection       GatewaySelection
	Origin          string
	SomIssueIDs     []string
	Nodes           []AgentNode
	Edges           []AgentEdge
}

type ImportStats struct {
	Events   int
	Entities int
	Nodes    int
	Edges    int
}

type EventFilter struct {
	EventType  *string
	SourceCode *string
	EntityID   *string
	From       *time.Time
	To         *time.Time
	Q          *string
	Cursor     *PageCursor
	Limit      int
}

type EventSummary struct {
	ID             string
	SourceCode     string
	SourceEventID  string
	SourceRef      *string
	Title          string
	EventType      string
	OccurredAt     time.Time
	IngestedAt     time.Time
	AttachedAt     time.Time
	AttachedBy     string
	Reason         *string
	NormalizedData []byte
}

type Event struct {
	EventSummary
	InvestigationIDs []string
	Entities         []EventEntity
}

type EventEntity struct {
	EntityID     string
	RelationCode string
}

type EntityFilter struct {
	TypeCode *string
	Q        *string
	Cursor   *PageCursor
	Limit    int
}

type Entity struct {
	ID           string
	TypeCode     string
	CanonicalKey string
	DisplayName  *string
	Metadata     []byte
	FirstSeen    *time.Time
	LastSeen     *time.Time
	AddedVia     *string
	AddedAt      time.Time
	Sources      []EntitySource
}

type EntitySource struct {
	SourceCode     string
	SourceEntityID string
	SourceRef      *string
	FetchedAt      time.Time
}

type EntityNew struct {
	ProjectID       string
	InvestigationID string
	TypeCode        string
	CanonicalKey    string
	DisplayName     *string
	Metadata        []byte
}

type EntityCard struct {
	Entity      Entity
	EventsCount int
	Occurrences []EntityOccurrence
	Neighbors   []EntityNeighbor
}

type EntityOccurrence struct {
	InvestigationID string
	Title           string
	EventsCount     int
}

type EntityNeighbor struct {
	EntityID     string
	DisplayName  *string
	RelationCode string
}

type NodeFilter struct {
	IncludeSubtree bool
	NodeType       *string
	Q              *string
	Cursor         *PageCursor
	Limit          int
}

type GraphNode struct {
	ID              string
	InvestigationID string
	NodeType        string
	EntityID        *string
	EventID         *string
	Origin          string
	SomIssueIDs     []string
	Label           *string
	TypeCode        *string
	CanonicalKey    *string
	OccurredAt      *time.Time
	CreatedAt       time.Time
}

type EdgeFilter struct {
	IncludeSubtree bool
	Statuses       []string
	Origin         *string
	RelationCode   *string
	NodeID         *string
	MinConfidence  *float32
	Cursor         *PageCursor
	Limit          int
}

type GraphEdge struct {
	ID               string
	InvestigationID  string
	SourceNodeID     string
	TargetNodeID     string
	RelationCode     string
	Status           string
	RejectReason     *string
	Confidence       *float32
	Why              *string
	Origin           string
	OriginRef        *string
	EvidenceEventIDs []string
	Metadata         []byte
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type EntityType struct{ Code, Title, Category string }
type RelationType struct {
	Code, Title, SourceKind, TargetKind string
	Directed                            bool
}
type Source struct{ Code, Kind, Title string }
type Reference struct {
	EntityTypes   []EntityType
	RelationTypes []RelationType
	Sources       []Source
}
