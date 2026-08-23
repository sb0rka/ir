package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Capability string

const (
	CapabilityEvents           Capability = "events"
	CapabilityEntityLookup     Capability = "entity_lookup"
	CapabilityArtifactAnalysis Capability = "artifact_analysis"
	CapabilityEndpoints        Capability = "endpoints"
	CapabilityResponseCatalog  Capability = "response_catalog"
	CapabilityAccountUserinfo  Capability = "account_userinfo"
)

type Source struct {
	Code         string
	Name         string
	Kind         string
	Mode         string
	Status       string
	Capabilities []Capability
}

type Provenance struct {
	Source     string
	ExternalID string
	SourceURL  string
	FetchedAt  time.Time
}

type EntityRef struct {
	Type  string
	Value string
}

type EventSourceRef struct {
	SourceCode    string
	SourceEventID string
}

type EntitySourceRef struct {
	SourceCode     string
	SourceEntityID string
}

type Entity struct {
	Type       string
	Value      string
	Attributes map[string]any
	Provenance []Provenance
}

type Event struct {
	Type       string
	Title      string
	Severity   string
	OccurredAt time.Time
	Entities   []EntityRef
	Attributes map[string]any
	Provenance Provenance
}

type Relation struct {
	Type         string
	SourceEntity EntityRef
	TargetEntity EntityRef
	OccurredAt   *time.Time
	Provenance   Provenance
}

type Hashes struct {
	MD5    string
	SHA1   string
	SHA256 string
}

type Artifact struct {
	ID     uuid.UUID
	Name   string
	MIME   string
	Size   int64
	Hashes Hashes
}

type Verdict struct {
	Value      string
	Confidence float64
	Labels     []string
	Provider   string
}

type Analysis struct {
	ID         uuid.UUID
	Status     string
	Artifact   Artifact
	Verdict    Verdict
	Artifacts  []Artifact
	Attributes map[string]any
	Provenance Provenance
}

type Endpoint struct {
	ID          uuid.UUID
	ExternalID  string
	Hostname    string
	IPAddresses []string
	Status      string
	Attributes  map[string]any
	Provenance  Provenance
}

type ResponseAction struct {
	Code        string
	Title       string
	Destructive bool
	Enabled     bool
}

type SourceError struct {
	Source    string
	Code      string
	Message   string
	Retryable bool
}

type AccountUserinfo struct {
	SourceCode string
	UserName   string
}

var idNamespace = uuid.MustParse("371be91f-7baf-5bb1-b576-9cd358848148")

func StableID(parts ...string) uuid.UUID {
	return uuid.NewSHA1(idNamespace, []byte(strings.Join(parts, "\x00")))
}

func CanonicalValue(kind, value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ip", "domain", "hostname", "email", "file_hash", "hash":
		return strings.ToLower(value)
	default:
		return value
	}
}

func NewEntity(kind, value string, provenance Provenance) Entity {
	kind = strings.ToLower(strings.TrimSpace(kind))
	value = CanonicalValue(kind, value)
	return Entity{
		Type:       strings.ToLower(strings.TrimSpace(kind)),
		Value:      value,
		Attributes: map[string]any{},
		Provenance: []Provenance{provenance},
	}
}
