package domain

import (
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Capability string

const (
	CapabilityFindings         Capability = "findings"
	CapabilitySessions         Capability = "sessions"
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

type EntityMention struct {
	EntityRef
	Roles []string
}

type TimeRange struct {
	From time.Time
	To   time.Time
}

type SourceObjectRef struct {
	SourceCode     string
	SourceInstance string
	RecordType     string
	ExternalID     string
	TimeRange      TimeRange
}

type RuleRef struct {
	ID   string
	Name string
}

type IncidentDetails struct {
	Key            string
	ExternalKey    string
	Type           string
	Verdict        string
	Damage         string
	Recommendation string
	AssignedTo     string
	ChangedAt      *time.Time
	Archived       bool
	Removed        bool
}

type CorrelationDetails struct {
	CorrelationType string
	SubeventCount   int
}

type NADAttackDetails struct {
	Class         string
	GID           int
	SID           int
	Revision      int
	RawPriority   int
	FalsePositive *bool
}

type Finding struct {
	Ref             SourceObjectRef
	Kind            string
	Title           string
	Description     string
	Severity        string
	OccurredAt      time.Time
	Status          string
	Rule            *RuleRef
	Entities        []EntityMention
	RelatedFindings []SourceObjectRef
	RelatedSessions []SourceObjectRef
	Incident        *IncidentDetails
	Correlation     *CorrelationDetails
	NADAttack       *NADAttackDetails
	SourceRef       string
	FetchedAt       time.Time
}

type NetworkEndpoint struct {
	IP   string
	MAC  string
	Host string
	Port int
}

type TrafficCounters struct {
	Sent     int64
	Received int64
	Total    int64
}

type SessionFileHint struct {
	ExternalID string
	Name       string
	MIME       string
	Size       int64
	MD5        string
	SHA256     string
	State      string
	Direction  string
}

type SessionAuthenticationHint struct {
	Protocol       string
	Method         string
	Account        string
	Valid          *bool
	FailedAttempts *int64
	ClientHost     string
	ServerHost     string
}

type Session struct {
	Ref                 SourceObjectRef
	Title               string
	Severity            string
	RawCriticality      *int
	StartedAt           time.Time
	EndedAt             *time.Time
	DurationSeconds     *float64
	SourceEndpoint      NetworkEndpoint
	DestinationEndpoint NetworkEndpoint
	TransportProtocol   string
	ApplicationProtocol string
	Bytes               *TrafficCounters
	Packets             *TrafficCounters
	State               []string
	FalsePositive       *bool
	HasFiles            *bool
	FileHints           []SessionFileHint
	AuthenticationHints []SessionAuthenticationHint
	TCPFlags            []string
	Entities            []EntityMention
	RelatedFindings     []SourceObjectRef
	SourceRef           string
	FetchedAt           time.Time
}

type SourceState struct {
	Source string
	Status string
	// Total is the vendor match count for this source when known.
	Total *int64
}

type ObjectResolution struct {
	Ref    SourceObjectRef
	Status string
	Errors []SourceError
}

type EventSourceRef struct {
	SourceCode    string
	SourceEventID string
}

type EventGroup struct {
	SourceCode string
	Values     []*string
	Count      int64
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
	Entities   []EntityMention
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
	case "ip":
		if parsed := net.ParseIP(value); parsed != nil {
			return parsed.String()
		}
		return strings.ToLower(value)
	case "mac":
		if parsed, err := net.ParseMAC(value); err == nil {
			return strings.ToLower(parsed.String())
		}
		return strings.ToLower(value)
	case "domain", "host", "hostname":
		return strings.TrimSuffix(strings.ToLower(value), ".")
	case "account", "email", "file_hash", "hash", "md5", "sha1", "sha256":
		if strings.EqualFold(strings.TrimSpace(kind), "account") {
			for strings.Contains(value, `\\`) {
				value = strings.ReplaceAll(value, `\\`, `\`)
			}
		}
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
