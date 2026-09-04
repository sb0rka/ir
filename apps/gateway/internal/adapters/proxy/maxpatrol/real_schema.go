package maxpatrol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type TimeRange struct {
	From time.Time
	To   time.Time
}

func (value TimeRange) validate() error {
	if value.From.IsZero() || value.To.IsZero() || !value.From.Before(value.To) {
		return &RequestError{Operation: "time range", Message: "from must be earlier than to"}
	}
	return nil
}

type EntityMention struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Role  string `json:"role"`
}

type IncidentSource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Assignee accepts the string and object forms used by Incident Read Model
// versions while retaining only a bounded identifier and display name.
type Assignee struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func (assignee *Assignee) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		assignee.Name = cleanText(name, maxNameLength)
		return nil
	}
	var value struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		UserName    string `json:"userName"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("invalid assignee")
	}
	assignee.ID = cleanIdentifier(value.ID)
	for _, candidate := range []string{value.Name, value.DisplayName, value.UserName} {
		if candidate = cleanText(candidate, maxNameLength); candidate != "" {
			assignee.Name = candidate
			break
		}
	}
	return nil
}

type Incident struct {
	ID              string         `json:"id"`
	Index           int64          `json:"index"`
	Key             string         `json:"key"`
	ExternalKey     string         `json:"externalKey"`
	ExternalID      string         `json:"externalId"`
	Name            string         `json:"name"`
	Source          IncidentSource `json:"source"`
	Severity        string         `json:"severity"`
	DetectedAt      time.Time      `json:"detectedAt"`
	Verdict         string         `json:"verdict"`
	Description     string         `json:"description"`
	Recommendation  string         `json:"recommendation"`
	Damage          string         `json:"damage"`
	Type            string         `json:"type"`
	AssignedTo      *Assignee      `json:"assignedTo"`
	State           string         `json:"state"`
	CreatedAt       time.Time      `json:"createdAt"`
	ChangedAt       time.Time      `json:"changedAt"`
	IsArchived      bool           `json:"isArchived"`
	IsRemoved       bool           `json:"isRemoved"`
	ParentID        *string        `json:"parentId"`
	IsParent        bool           `json:"isParent"`
	HasNotification bool           `json:"hasNotification"`
}

type IncidentSearchRequest struct {
	TimeRange TimeRange
	Limit     int
	Offset    int
}

type IncidentPage struct {
	Incidents  []Incident
	TotalItems int
	Offset     int
	NextOffset *int
	Truncated  bool
}

type IncidentAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type IncidentHost struct {
	ID      string  `json:"id"`
	AssetID *string `json:"assetId"`
	FQDN    string  `json:"fqdn"`
	IP      *string `json:"ip"`
	Role    string  `json:"role"`
}

type IncidentFile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	MD5    string `json:"md5"`
	SHA1   string `json:"sha1"`
	SHA256 string `json:"sha256"`
}

type IncidentExternalLink struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Internal bool   `json:"internal"`
}

type IncidentAssetGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ProcessHint struct {
	Name        string `json:"name,omitempty"`
	Path        string `json:"path,omitempty"`
	CommandLine string `json:"command_line,omitempty"`
}

// safeEventRecord is a strict allowlist. Incident fullInfo can contain large
// rule tables, raw payload material, and product-internal metadata; all fields
// not listed here are discarded during JSON decoding.
type safeEventRecord struct {
	Time                        time.Time `json:"time"`
	UUID                        string    `json:"uuid"`
	Text                        string    `json:"text"`
	Action                      string    `json:"action"`
	Importance                  string    `json:"importance"`
	CorrelationName             string    `json:"correlation_name"`
	CorrelationType             string    `json:"correlation_type"`
	SubeventCount               int       `json:"count.subevents"`
	SubeventIDs                 []string  `json:"subevents"`
	AlertContext                string    `json:"alert.context"`
	AlertKey                    string    `json:"alert.key"`
	AlertRegexMatch             string    `json:"alert.regex_match"`
	EventSourceHost             string    `json:"event_src.host"`
	EventSourceFQDN             string    `json:"event_src.fqdn"`
	EventSourceHostname         string    `json:"event_src.hostname"`
	EventSourceIP               string    `json:"event_src.ip"`
	EventSourceMAC              string    `json:"event_src.mac"`
	EventSourceVendor           string    `json:"event_src.vendor"`
	EventSourceTitle            string    `json:"event_src.title"`
	EventSourceSubsystem        string    `json:"event_src.subsys"`
	SourceHost                  string    `json:"src.host"`
	SourceHostname              string    `json:"src.hostname"`
	SourceIP                    string    `json:"src.ip"`
	SourceMAC                   string    `json:"src.mac"`
	SourcePort                  int64     `json:"src.port"`
	DestinationHost             string    `json:"dst.host"`
	DestinationHostname         string    `json:"dst.hostname"`
	DestinationFQDN             string    `json:"dst.fqdn"`
	DestinationIP               string    `json:"dst.ip"`
	DestinationMAC              string    `json:"dst.mac"`
	DestinationPort             int64     `json:"dst.port"`
	ExternalDestinationHost     string    `json:"external_dst.host"`
	ExternalDestinationHostname string    `json:"external_dst.hostname"`
	ExternalDestinationFQDN     string    `json:"external_dst.fqdn"`
	Subject                     string    `json:"subject"`
	SubjectAccountName          string    `json:"subject.account.name"`
	SubjectAccountDomain        string    `json:"subject.account.domain"`
	SubjectAccountProvider      string    `json:"subject.account.provider"`
	SubjectAccountSessionID     string    `json:"subject.account.session_id"`
	SubjectAccountID            string    `json:"subject.account.id"`
	ObjectAccountName           string    `json:"object.account.name"`
	ObjectAccountDomain         string    `json:"object.account.domain"`
	ObjectAccountID             string    `json:"object.account.id"`
	SubjectProcessName          string    `json:"subject.process.name"`
	SubjectProcessPath          string    `json:"subject.process.fullpath"`
	SubjectProcessCommand       string    `json:"subject.process.cmdline"`
	SubjectProcessID            string    `json:"subject.process.id"`
	SubjectProcessChain         string    `json:"subject.process.chain"`
	ObjectProcessName           string    `json:"object.process.name"`
	ObjectProcessPath           string    `json:"object.process.fullpath"`
	ObjectProcessCommand        string    `json:"object.process.cmdline"`
	ObjectProcessID             string    `json:"object.process.id"`
	ObjectProcessChain          string    `json:"object.process.chain"`
	Object                      string    `json:"object"`
	ObjectName                  string    `json:"object.name"`
	ObjectPath                  string    `json:"object.path"`
	CategoryGeneric             string    `json:"category.generic"`
	CategoryHigh                string    `json:"category.high"`
	CategoryLow                 string    `json:"category.low"`
}

type incidentEventFullInfo safeEventRecord

func (info *incidentEventFullInfo) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if len(data) >= 2 && data[0] == '"' {
		var encoded string
		if err := json.Unmarshal(data, &encoded); err != nil {
			return err
		}
		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return nil
		}
		var record safeEventRecord
		if err := json.Unmarshal([]byte(encoded), &record); err != nil {
			return err
		}
		*info = incidentEventFullInfo(record)
		return nil
	}
	var record safeEventRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	*info = incidentEventFullInfo(record)
	return nil
}

func (info incidentEventFullInfo) record() safeEventRecord {
	return safeEventRecord(info)
}

type IncidentEvent struct {
	ID          string                `json:"id"`
	ExternalID  string                `json:"externalId"`
	EventKey    string                `json:"eventKey"`
	Description string                `json:"description"`
	DetectedAt  time.Time             `json:"detectedAt"`
	Type        string                `json:"type"`
	FullInfo    incidentEventFullInfo `json:"fullInfo"`
}

type Correlation struct {
	UUID            string
	RuleName        string
	CorrelationType string
	Title           string
	Severity        string
	OccurredAt      time.Time
	Action          string
	SubeventCount   int
	SubeventIDs     []string
	Entities        []EntityMention
}

type RawEvent struct {
	UUID            string
	Title           string
	Severity        string
	OccurredAt      time.Time
	Action          string
	EventSourceHost string
	EventSourceIP   string
	SourceHost      string
	SourceIP        string
	SourcePort      int64
	DestinationHost string
	DestinationIP   string
	DestinationPort int64
	ActorAccount    string
	ObjectAccount   string
	SubjectProcess  ProcessHint
	ObjectProcess   ProcessHint
	ObjectName      string
	ObjectPath      string
	CategoryGeneric string
	CategoryHigh    string
	CategoryLow     string
	Entities        []EntityMention
}

type CorrelationSearchRequest struct {
	TimeRange TimeRange
	Limit     int
}

type CorrelationPage struct {
	Correlations  []Correlation
	TotalItems    int64
	ReportedTotal *int64
	Truncated     bool
}

type CorrelationResolveRequest struct {
	ExternalID string
	TimeRange  TimeRange
}

type CorrelationResolution struct {
	Correlation Correlation
	Subevents   []RawEvent
	Complete    bool
	Errors      []ContextError
}

type IncidentResolveRequest struct {
	ExternalID string
	TimeRange  TimeRange
}

type IncidentResolution struct {
	Incident     Incident
	Correlations []CorrelationResolution
	Events       []RawEvent
	Accounts     []IncidentAccount
	Hosts        []IncidentHost
	Files        []IncidentFile
	Links        []IncidentExternalLink
	AssetGroups  []IncidentAssetGroup
	Complete     bool
	Truncated    bool
	Errors       []ContextError
}

type incidentBooleanFilter struct {
	Value string `json:"value"`
}

type incidentTimeFilter struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type incidentListFilter struct {
	DetectedAt incidentTimeFilter    `json:"detectedAt"`
	IsRemoved  incidentBooleanFilter `json:"isRemoved"`
	IsArchived incidentBooleanFilter `json:"isArchived"`
}

type incidentListPayload struct {
	Filter  incidentListFilter `json:"filter"`
	Sorting []struct{}         `json:"sorting"`
}

type incidentListEnvelope struct {
	Incidents  []Incident `json:"incidents"`
	TotalItems int        `json:"totalItems"`
}

type incidentChildrenEnvelope struct {
	Events     []IncidentEvent        `json:"events"`
	Accounts   []IncidentAccount      `json:"accounts"`
	Files      []IncidentFile         `json:"files"`
	Links      []IncidentExternalLink `json:"links"`
	Items      []IncidentAssetGroup   `json:"items"`
	TotalItems int                    `json:"totalItems"`
}

type eventQueryV3Request struct {
	Filter      string   `json:"filter"`
	GroupValues []string `json:"groupValues"`
	TimeFrom    int64    `json:"timeFrom"`
	TimeTo      int64    `json:"timeTo"`
}

type eventQueryRequest struct {
	Filter      eventQueryFilter `json:"filter"`
	GroupValues []string         `json:"groupValues"`
	TimeFrom    int64            `json:"timeFrom"`
	TimeTo      int64            `json:"timeTo"`
}

type eventQueryFilter struct {
	AggregateBy         []struct{}        `json:"aggregateBy"`
	Aliases             eventQueryAliases `json:"aliases"`
	DistributeBy        []string          `json:"distributeBy"`
	GroupBy             []string          `json:"groupBy"`
	GroupByOrder        []OrderBy         `json:"groupByOrder"`
	LocalSources        []string          `json:"localSources"`
	LocalSourcesAliases []string          `json:"localSourcesAliases"`
	OrderBy             []OrderBy         `json:"orderBy"`
	SearchAliasNames    []string          `json:"searchAliasNames"`
	Select              []string          `json:"select"`
	ShowNullGroups      bool              `json:"showNullGroups"`
	Top                 *int              `json:"top"`
	Where               string            `json:"where"`
}

type eventQueryAliases struct {
	AggregateBy map[string]string `json:"aggregateBy"`
	GroupBy     map[string]string `json:"groupBy"`
}

type safeEventsEnvelope struct {
	Errors     json.RawMessage   `json:"errors"`
	Events     []safeEventRecord `json:"events"`
	Token      string            `json:"token"`
	TotalCount int64             `json:"totalCount"`
	// ReportedTotal is the authentic vendor match count when known. It is not
	// JSON-backed: TotalCount may be filled with len(events) for internal checks
	// when noCount omitted a real total.
	ReportedTotal *int64 `json:"-"`
}

// authenticMatchTotal returns a pointer to the vendor match count when SIEM
// reported one. TotalCount == 0 with a non-empty page is treated as unknown
// (typical noCount response without totalCount).
func authenticMatchTotal(totalCount int64, returned int) *int64 {
	if totalCount > 0 {
		value := totalCount
		return &value
	}
	if returned == 0 {
		zero := int64(0)
		return &zero
	}
	return nil
}

func finalizeEventsEnvelope(response *safeEventsEnvelope) error {
	response.ReportedTotal = authenticMatchTotal(response.TotalCount, len(response.Events))
	if response.TotalCount <= 0 {
		response.TotalCount = int64(len(response.Events))
	}
	if int64(len(response.Events)) > response.TotalCount {
		return fmt.Errorf("pagination metadata is inconsistent")
	}
	return nil
}

func vendorTime(value time.Time) string {
	// Incident Read Model captures use millisecond request boundaries.
	return value.Format("2006-01-02T15:04:05.000Z07:00")
}

func intQuery(value int) string { return strconv.Itoa(value) }

func nonNullVendorErrors(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "[]" && trimmed != "{}"
}
