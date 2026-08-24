package ptnad

import (
	"errors"
	"fmt"
	"time"
)

const (
	SourceCode        = "pt-nad"
	SessionRecordType = "nad_session"
	AttackRecordType  = "nad_attack"
	DefaultLimit      = 5000
	MaxLimit          = 5000
)

var ErrNotFound = errors.New("PT NAD record not found")

// Access contains the rotating credential supplied by the project-scoped
// credential cache. It is intentionally passed per request and is never stored
// on Client.
type Access struct {
	Cookie string
}

type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type SearchRequest struct {
	StoreID int64
	From    time.Time
	To      time.Time
	Limit   int
}

type SessionRef struct {
	StoreID    int64
	ExternalID string
	TimeRange  TimeRange
}

type AttackRef struct {
	StoreID    int64
	ExternalID string
	TimeRange  TimeRange
}

// SourceRef is the stable, store-namespaced PT NAD identity. TimeRange is
// replay provenance and is not part of identity.
type SourceRef struct {
	SourceCode     string    `json:"source_code"`
	SourceInstance string    `json:"source_instance"`
	RecordType     string    `json:"record_type"`
	ExternalID     string    `json:"external_id"`
	TimeRange      TimeRange `json:"time_range"`
}

func (ref SourceRef) Identity() string {
	return ref.SourceCode + "\x00" + ref.SourceInstance + "\x00" + ref.RecordType + "\x00" + ref.ExternalID
}

type SessionSearchResult struct {
	Sessions  []Session `json:"sessions"`
	Total     int64     `json:"total"`
	Truncated bool      `json:"truncated"`
}

type AttackSearchResult struct {
	Attacks   []Attack `json:"attacks"`
	Total     int64    `json:"total"`
	Truncated bool     `json:"truncated"`
}

type Endpoint struct {
	IP      string   `json:"ip,omitempty"`
	MAC     string   `json:"mac,omitempty"`
	Host    string   `json:"host,omitempty"`
	HostID  string   `json:"host_id,omitempty"`
	DNS     string   `json:"dns,omitempty"`
	Port    int64    `json:"port,omitempty"`
	Country string   `json:"country,omitempty"`
	Groups  []string `json:"groups,omitempty"`
}

type Counters struct {
	Received int64 `json:"received"`
	Sent     int64 `json:"sent"`
	Total    int64 `json:"total"`
}

type Session struct {
	SourceRef            SourceRef            `json:"source_ref"`
	FetchedAt            time.Time            `json:"fetched_at"`
	Start                time.Time            `json:"start"`
	End                  time.Time            `json:"end"`
	DurationSeconds      float64              `json:"duration_seconds"`
	Source               Endpoint             `json:"source"`
	Destination          Endpoint             `json:"destination"`
	TransportProtocol    string               `json:"transport_protocol,omitempty"`
	ApplicationProtocol  string               `json:"application_protocol,omitempty"`
	ApplicationProtocols []string             `json:"application_protocols,omitempty"`
	Bytes                Counters             `json:"bytes"`
	Packets              Counters             `json:"packets"`
	State                []string             `json:"state,omitempty"`
	Errors               []string             `json:"errors,omitempty"`
	TCPFlags             []string             `json:"tcp_flags,omitempty"`
	TCPFlagsClient       []string             `json:"tcp_flags_client,omitempty"`
	TCPFlagsServer       []string             `json:"tcp_flags_server,omitempty"`
	Criticality          *int64               `json:"criticality,omitempty"`
	Severity             string               `json:"severity"`
	HasFiles             bool                 `json:"has_files"`
	FalsePositive        *bool                `json:"false_positive,omitempty"`
	StoreTag             string               `json:"store_tag,omitempty"`
	StorageIndex         string               `json:"storage_index,omitempty"`
	Banners              Banners              `json:"banners,omitempty"`
	OperatingSystems     OperatingSystems     `json:"operating_systems,omitempty"`
	Files                []FileHint           `json:"files,omitempty"`
	Authentication       []AuthenticationHint `json:"authentication,omitempty"`
	RelatedAttacks       []Attack             `json:"related_attacks,omitempty"`
	SSH                  []SSHHint            `json:"ssh,omitempty"`
	HTTP                 []HTTPHint           `json:"http,omitempty"`
	SMB                  []SMBHint            `json:"smb,omitempty"`
	DCERPC               []DCERPCHint         `json:"dcerpc,omitempty"`
	NTLM                 []NTLMHint           `json:"ntlm,omitempty"`
}

type Attack struct {
	SourceRef      SourceRef  `json:"source_ref"`
	FetchedAt      time.Time  `json:"fetched_at"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	Recommendation string     `json:"recommendation,omitempty"`
	Class          string     `json:"class,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
	Severity       string     `json:"severity"`
	RawPriority    *int64     `json:"raw_priority,omitempty"`
	GID            int64      `json:"gid,omitempty"`
	SID            int64      `json:"sid,omitempty"`
	Revision       int64      `json:"revision,omitempty"`
	FalsePositive  *bool      `json:"false_positive,omitempty"`
	Attacker       Endpoint   `json:"attacker"`
	Victim         Endpoint   `json:"victim"`
	ParentSession  *SourceRef `json:"parent_session,omitempty"`
	RuleVendor     string     `json:"rule_vendor,omitempty"`
	AttackTarget   string     `json:"attack_target,omitempty"`
	AttackFlag     *bool      `json:"attack_flag,omitempty"`
	RuleDisabled   *bool      `json:"rule_disabled,omitempty"`
	MatchType      string     `json:"match_type,omitempty"`
	ATTACK         []string   `json:"attack,omitempty"`
	Direction      string     `json:"direction,omitempty"`
}

type FileHint struct {
	ExternalID string `json:"external_id"`
	ParentID   string `json:"parent_id"`
	VendorID   int64  `json:"vendor_id"`
	TxID       *int64 `json:"transaction_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Path       string `json:"path,omitempty"`
	Magic      string `json:"magic,omitempty"`
	MIME       string `json:"mime,omitempty"`
	MD5        string `json:"md5,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Size       int64  `json:"size"`
	State      string `json:"state,omitempty"`
	Direction  string `json:"direction,omitempty"`
}

type AuthenticationHint struct {
	Protocol   string `json:"protocol"`
	Account    string `json:"account,omitempty"`
	Valid      *bool  `json:"valid,omitempty"`
	ClientHost string `json:"client_host,omitempty"`
	ServerHost string `json:"server_host,omitempty"`
}

type Banners struct {
	Client []string `json:"client,omitempty"`
	Server []string `json:"server,omitempty"`
}

type OperatingSystems struct {
	Client []string `json:"client,omitempty"`
	Server []string `json:"server,omitempty"`
}

type TransactionRef struct {
	ExternalID string    `json:"external_id"`
	TxID       int64     `json:"transaction_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type SSHHint struct {
	Transaction         TransactionRef `json:"transaction"`
	Authentication      string         `json:"authentication,omitempty"`
	FailedPasswordCount *int64         `json:"failed_password_count,omitempty"`
	KeyPressed          *bool          `json:"key_pressed,omitempty"`
	Compression         []string       `json:"compression,omitempty"`
	Encryption          []string       `json:"encryption,omitempty"`
	ClientProtocol      string         `json:"client_protocol,omitempty"`
	ClientSoftware      string         `json:"client_software,omitempty"`
	ServerProtocol      string         `json:"server_protocol,omitempty"`
	ServerSoftware      string         `json:"server_software,omitempty"`
}

type HTTPHint struct {
	Transaction         TransactionRef `json:"transaction"`
	Method              string         `json:"method,omitempty"`
	Path                string         `json:"path,omitempty"`
	NormalizedPath      string         `json:"normalized_path,omitempty"`
	Host                string         `json:"host,omitempty"`
	Protocol            string         `json:"protocol,omitempty"`
	RequestBytes        int64          `json:"request_bytes"`
	RequestContentType  string         `json:"request_content_type,omitempty"`
	ResponseCode        int64          `json:"response_code,omitempty"`
	ResponseStatus      string         `json:"response_status,omitempty"`
	ResponseBytes       int64          `json:"response_bytes"`
	ResponseServer      string         `json:"response_server,omitempty"`
	ResponseContentType string         `json:"response_content_type,omitempty"`
}

type SMBHint struct {
	Transaction TransactionRef `json:"transaction"`
	Command     string         `json:"command,omitempty"`
	Status      string         `json:"status,omitempty"`
	Filename    string         `json:"filename,omitempty"`
	Action      string         `json:"action,omitempty"`
	TreePath    string         `json:"tree_path,omitempty"`
	ShareType   string         `json:"share_type,omitempty"`
}

type DCERPCHint struct {
	Transaction      TransactionRef `json:"transaction"`
	PacketType       string         `json:"packet_type,omitempty"`
	Interface        string         `json:"interface,omitempty"`
	Operation        string         `json:"operation,omitempty"`
	AuthType         string         `json:"auth_type,omitempty"`
	AuthLevel        string         `json:"auth_level,omitempty"`
	ArgumentsDecoded bool           `json:"arguments_decoded"`
}

type NTLMHint struct {
	Transaction TransactionRef `json:"transaction"`
	MessageType string         `json:"message_type,omitempty"`
	Account     string         `json:"account,omitempty"`
	ClientHost  string         `json:"client_host,omitempty"`
	TargetHost  string         `json:"target_host,omitempty"`
	Domain      string         `json:"domain,omitempty"`
	OSVersion   string         `json:"os_version,omitempty"`
	OSBuild     int64          `json:"os_build,omitempty"`
}

type Store struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	LastImport  time.Time `json:"last_import,omitempty"`
	Created     time.Time `json:"created,omitempty"`
	Modified    time.Time `json:"modified,omitempty"`
	FilesCount  int64     `json:"files_count"`
	Volume      int64     `json:"volume"`
	IsLive      bool      `json:"is_live"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// ResponseError contains only safe request metadata. Vendor response bodies,
// URLs, queries, and credentials are intentionally excluded.
type ResponseError struct {
	Operation  string
	StatusCode int
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("PT NAD %s returned HTTP %d", err.Operation, err.StatusCode)
}

// TransportError deliberately does not unwrap net/url errors because their
// text contains the credentialed request URL and its store/time query.
type TransportError struct {
	Operation        string
	TimedOut         bool
	TemporaryFailure bool
}

func (err *TransportError) Error() string {
	return fmt.Sprintf("PT NAD %s transport failed", err.Operation)
}

func (err *TransportError) Timeout() bool   { return err.TimedOut }
func (err *TransportError) Temporary() bool { return err.TemporaryFailure }

// ProtocolError reports an invalid bounded vendor response without quoting it.
type ProtocolError struct {
	Operation string
}

func (err *ProtocolError) Error() string {
	return fmt.Sprintf("PT NAD %s response is invalid", err.Operation)
}
