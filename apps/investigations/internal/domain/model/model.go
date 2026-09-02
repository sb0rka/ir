// Package model contains transport-independent investigation domain values.
package model

import "time"

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
	Findings      int
	Sessions      int
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

type InvestigationPatch struct {
	ProjectID       string
	InvestigationID string
	Version         int
	Title           *string
	Description     *string
	Status          *string
	Verdict         *string
	VerdictReason   *string
	Confidence      *float32
	Severity        *string
	WorkspaceIDs    *[]string
}

type HypothesisNew struct {
	ProjectID       string
	InvestigationID string
	Statement       string
	Description     *string
}

type Hypothesis struct {
	ID              string
	ProjectID       string
	InvestigationID string
	Statement       string
	Description     *string
	Status          string
	Reason          *string
	Origin          string
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ResolvedAt      *time.Time
}

type HypothesisPatch struct {
	ProjectID       string
	InvestigationID string
	HypothesisID    string
	Version         int
	Statement       *string
	Description     *string
	HasDescription  bool
	Status          *string
	Reason          *string
}

type HypothesisFilter struct {
	Status *string
	Cursor *PageCursor
	Limit  int
}

type HypothesisGraph struct {
	Nodes []GraphNode
	Edges []GraphEdge
}

type GatewayProvenance struct {
	Source     string    `json:"source"`
	ExternalID string    `json:"external_id"`
	SourceURL  *string   `json:"source_url,omitempty"`
	FetchedAt  time.Time `json:"fetched_at"`
}

type GatewayEvent struct {
	SnapshotID string
	Direct     bool
	Title      string
	EventType  string
	Severity   string
	OccurredAt time.Time
	Entities   []GatewayEventEntity
	Attributes map[string]any
	Provenance GatewayProvenance
}

type GatewayEventEntity struct {
	SnapshotID string
	Roles      []string
}

type GatewayEntity struct {
	SnapshotID string
	Direct     bool
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

type GatewayTimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type GatewayObjectRef struct {
	SourceCode     string           `json:"source_code"`
	SourceInstance string           `json:"source_instance,omitempty"`
	RecordType     string           `json:"record_type"`
	ExternalID     string           `json:"external_id"`
	TimeRange      GatewayTimeRange `json:"time_range"`
}

type GatewayContextError struct {
	Source    string `json:"source"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type GatewayFinding struct {
	SnapshotID        string
	Ref               GatewayObjectRef
	Kind              string
	Title             string
	Description       *string
	Severity          string
	OccurredAt        time.Time
	Status            *string
	SourceRef         *string
	FetchedAt         time.Time
	Normalized        []byte
	Provenance        []byte
	ContextStatus     string
	ContextErrors     []GatewayContextError
	Direct            bool
	EventSnapshotIDs  []string
	EntitySnapshotIDs []string
	RelatedFindings   []GatewayObjectRef
	RelatedSessions   []GatewayObjectRef
}

type GatewaySession struct {
	SnapshotID        string
	Ref               GatewayObjectRef
	Title             string
	Severity          string
	StartedAt         time.Time
	EndedAt           *time.Time
	SourceRef         *string
	FetchedAt         time.Time
	Normalized        []byte
	Provenance        []byte
	ContextStatus     string
	ContextErrors     []GatewayContextError
	Direct            bool
	EventSnapshotIDs  []string
	EntitySnapshotIDs []string
	RelatedFindings   []GatewayObjectRef
}

type GatewaySelection struct {
	Findings  []GatewayFinding
	Sessions  []GatewaySession
	Events    []GatewayEvent
	Entities  []GatewayEntity
	Relations []GatewayRelation
}

type AgentNode struct {
	Ref              string
	Why              string
	SnapshotEventID  *string
	SnapshotEntityID *string
	EventID          *string
	EntityID         *string
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
	ProjectID               string
	InvestigationID         string
	HypothesisID            *string
	RequireActiveHypothesis bool
	Selection               GatewaySelection
	Origin                  string
	SomIssueIDs             []string
	Nodes                   []AgentNode
	Edges                   []AgentEdge
	Warnings                []string
	Seed                    bool
	EntityGroupProposals    []GroupProposal
	EventGroupProposals     []GroupProposal
}

type ImportStats struct {
	Groups   []GroupImportResult
	Findings int
	Sessions int
	Events   int
	Entities int
	Nodes    int
	Edges    int
	Warnings []string
}

type ObjectFilter struct {
	RecordType    *string
	Severity      *string
	ContextStatus *string
	Cursor        *PageCursor
	Limit         int
}

type Finding struct {
	ID               string
	ProjectID        string
	Ref              GatewayObjectRef
	Kind             string
	Title            string
	Description      *string
	Severity         string
	OccurredAt       time.Time
	Status           *string
	SourceRef        *string
	FetchedAt        time.Time
	Normalized       []byte
	Provenance       []byte
	ContextStatus    string
	ContextErrors    []byte
	InvestigationIDs []string
	Direct           bool
	Derived          bool
	AttachedAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type NetworkSession struct {
	ID               string
	ProjectID        string
	Ref              GatewayObjectRef
	Title            string
	Severity         string
	StartedAt        time.Time
	EndedAt          *time.Time
	SourceRef        *string
	FetchedAt        time.Time
	Normalized       []byte
	Provenance       []byte
	ContextStatus    string
	ContextErrors    []byte
	InvestigationIDs []string
	Direct           bool
	Derived          bool
	AttachedAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
	IsSeed         bool
	NormalizedData []byte
	Entities       []EventEntity
}

type Event struct {
	EventSummary
	InvestigationIDs []string
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
	Why             *string
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

type GraphEdgeNew struct {
	ProjectID        string
	InvestigationID  string
	SourceNodeID     string
	TargetNodeID     string
	RelationCode     string
	Confidence       *float32
	Why              *string
	OriginRef        *string
	EvidenceEventIDs []string
}

type GraphEdgePatch struct {
	ProjectID       string
	InvestigationID string
	EdgeID          string
	Version         int
	Status          *string
	RejectReason    *string
	Confidence      *float32
	Why             *string
	Metadata        []byte
	HasMetadata     bool
}

type EvidenceEvent struct {
	ID            string
	SourceCode    string
	SourceEventID string
	SourceRef     *string
	EventType     string
	OccurredAt    time.Time
}

type EdgeReviewItem struct {
	ID      string
	Version int
	Reason  *string
}

type EdgeReviewRequest struct {
	ProjectID       string
	InvestigationID string
	Confirm         []EdgeReviewItem
	Reject          []EdgeReviewItem
}

type EdgeReviewResult struct {
	Confirmed []string
	Rejected  []string
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
