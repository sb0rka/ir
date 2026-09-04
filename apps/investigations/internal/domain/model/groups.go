package model

import "time"

// GroupScope is resolved from the investigation hierarchy, never from a proposal.
type GroupScope struct {
	ProjectID string `json:"project_id"`
	RootID    string `json:"root_investigation_id"`
}

type Group struct {
	ID string `json:"id"`
	GroupScope
	Family       string        `json:"family"`
	Kind         string        `json:"kind"`
	TypeCode     string        `json:"type_code,omitempty"`
	Key          string        `json:"group_key"`
	Title        string        `json:"title"`
	State        string        `json:"state"`
	Version      int           `json:"version"`
	Members      []GroupMember `json:"members"`
	SuccessorIDs []string      `json:"successor_ids"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type GroupMember struct {
	ID         string           `json:"id"`
	ObjectID   string           `json:"object_id"`
	Role       string           `json:"role"`
	Ordinal    *int             `json:"ordinal,omitempty"`
	Status     string           `json:"status"`
	Confidence *float32         `json:"confidence,omitempty"`
	Reason     string           `json:"decision_reason"`
	Version    int              `json:"version"`
	Assertions []GroupAssertion `json:"assertions"`
}

// Assertions retain separate observed intervals, rather than filling gaps with min/max.
type GroupAssertion struct {
	InvestigationID  string     `json:"investigation_id"`
	Origin           string     `json:"origin"`
	OriginRef        string     `json:"origin_ref"`
	Method           string     `json:"method"`
	MethodVersion    string     `json:"method_version"`
	EvidenceEventIDs []string   `json:"evidence_event_ids"`
	ValidFrom        *time.Time `json:"valid_from,omitempty"`
	ValidTo          *time.Time `json:"valid_to,omitempty"`
	Reason           string     `json:"reason"`
}

type GroupProposal struct {
	ProposalID        string                `json:"proposal_id"`
	Kind              string                `json:"kind"`
	TypeCode          string                `json:"type_code,omitempty"`
	Title             string                `json:"title"`
	Why               string                `json:"why"`
	Members           []GroupProposalMember `json:"members"`
	EvidenceEventRefs []string              `json:"evidence_event_refs"`
}

type GroupProposalMember struct {
	NodeRef    string     `json:"node_ref"`
	Role       string     `json:"role"`
	Ordinal    *int       `json:"ordinal,omitempty"`
	Confidence *float32   `json:"confidence,omitempty"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidTo    *time.Time `json:"valid_to,omitempty"`
}

type GroupImportResult struct {
	GroupID   string   `json:"group_id"`
	Family    string   `json:"family"`
	RootID    string   `json:"root_investigation_id"`
	MemberIDs []string `json:"member_ids"`
}

type GroupReview struct {
	OperationID string              `json:"operation_id"`
	Version     int                 `json:"version"`
	Reason      string              `json:"reason"`
	Members     []GroupReviewMember `json:"members"`
}

type GroupReviewMember struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Status  string `json:"status"`
}

type GroupVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type GroupPlacement struct {
	MemberID string `json:"member_id"`
	Role     string `json:"role"`
	Ordinal  *int   `json:"ordinal,omitempty"`
}

type GroupMerge struct {
	OperationID string           `json:"operation_id"`
	Version     int              `json:"version"`
	Reason      string           `json:"reason"`
	Sources     []GroupVersion   `json:"sources"`
	Members     []GroupPlacement `json:"members"`
}

type GroupPartition struct {
	Title   string           `json:"title"`
	Members []GroupPlacement `json:"members"`
}

type GroupSplit struct {
	OperationID string           `json:"operation_id"`
	Version     int              `json:"version"`
	Reason      string           `json:"reason"`
	Partitions  []GroupPartition `json:"partitions"`
}

type GroupMutation struct {
	GroupScope
	Family  string
	GroupID string
	Actor   string
	Review  *GroupReview
	Merge   *GroupMerge
	Split   *GroupSplit
}

type GroupOperation struct {
	ID string `json:"id"`
	GroupScope
	OperationID string    `json:"operation_id"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Reason      string    `json:"reason"`
	Before      []Group   `json:"before"`
	After       []Group   `json:"after"`
	CreatedAt   time.Time `json:"created_at"`
}

type GroupHistory struct {
	Operations []GroupOperation `json:"operations"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}

type ProjectionRequest struct {
	ProjectID       string
	InvestigationID string
	HypothesisID    *string
	Filter          EdgeFilter
}

type GraphProjection struct {
	InvestigationID string                 `json:"investigation_id"`
	RootID          string                 `json:"root_investigation_id"`
	HypothesisID    *string                `json:"hypothesis_id,omitempty"`
	IncludeSubtree  bool                   `json:"include_subtree"`
	Nodes           []ProjectionNode       `json:"nodes"`
	Edges           []ProjectionEdge       `json:"edges"`
	Annotations     []ProjectionAnnotation `json:"annotations"`
	Diagnostics     []ProjectionDiagnostic `json:"diagnostics"`
	Groups          []ProjectionGroup      `json:"groups"`
	// Raw objects are restricted to the requested view; they make expansion lossless.
	RawNodes []ProjectionRawNode `json:"raw_nodes"`
	RawEdges []ProjectionRawEdge `json:"raw_edges"`
}

// View-scoped memberships make proposals discoverable without collapsing them
// or exposing a whole-tree label/review reason that may mention hidden evidence.
type ProjectionGroup struct {
	ID      string                  `json:"id"`
	Family  string                  `json:"family"`
	Kind    string                  `json:"kind"`
	Version int                     `json:"version"`
	Members []ProjectionGroupMember `json:"members"`
}

type ProjectionGroupMember struct {
	ID         string           `json:"id"`
	NodeIDs    []string         `json:"node_ids"`
	Role       string           `json:"role"`
	Ordinal    *int             `json:"ordinal,omitempty"`
	Status     string           `json:"status"`
	Version    int              `json:"version"`
	Confidence *float32         `json:"confidence,omitempty"`
	Assertions []GroupAssertion `json:"assertions"`
}

type ProjectionRawNode struct {
	ID              string     `json:"id"`
	InvestigationID string     `json:"investigation_id"`
	NodeType        string     `json:"node_type"`
	EntityID        *string    `json:"entity_id,omitempty"`
	EventID         *string    `json:"event_id,omitempty"`
	Label           *string    `json:"label,omitempty"`
	TypeCode        *string    `json:"type_code,omitempty"`
	Origin          string     `json:"origin"`
	SomIssueIDs     []string   `json:"som_issue_ids"`
	OccurredAt      *time.Time `json:"occurred_at,omitempty"`
}

type ProjectionRawEdge struct {
	ID               string   `json:"id"`
	InvestigationID  string   `json:"investigation_id"`
	Source           string   `json:"source_node_id"`
	Target           string   `json:"target_node_id"`
	Relation         string   `json:"relation_code"`
	Status           string   `json:"status"`
	Origin           string   `json:"origin"`
	Confidence       *float32 `json:"confidence,omitempty"`
	Why              *string  `json:"why,omitempty"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
}

type ProjectionNode struct {
	ID            string   `json:"id"`
	NodeType      string   `json:"node_type"`
	GroupID       *string  `json:"group_id,omitempty"`
	GroupKind     *string  `json:"group_kind,omitempty"`
	MemberNodeIDs []string `json:"member_node_ids"`
}

type ProjectionEdge struct {
	ID               string   `json:"id"`
	Source           string   `json:"source_node_id"`
	Target           string   `json:"target_node_id"`
	Relation         string   `json:"relation_code"`
	Status           string   `json:"status"`
	Origins          []string `json:"origins"`
	MemberEdgeIDs    []string `json:"member_edge_ids"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
	ConfidenceMin    *float32 `json:"confidence_min,omitempty"`
	ConfidenceMax    *float32 `json:"confidence_max,omitempty"`
}

type ProjectionAnnotation struct {
	GroupID       string   `json:"group_id"`
	Kind          string   `json:"kind"`
	MemberNodeIDs []string `json:"member_node_ids"`
}

type ProjectionDiagnostic struct {
	Code    string   `json:"code"`
	NodeIDs []string `json:"node_ids"`
}
