package ptnad

import (
	"encoding/json"
	"fmt"
)

// PT NAD BQL returns positional rows. These DTOs deliberately model only the
// fixed select lists emitted by this package; callers cannot supply BQL or a
// column list.
type sessionListResponse struct {
	Took   int64            `json:"took"`
	Total  int64            `json:"total"`
	Result []sessionListRow `json:"result"`
}

type sessionListRow struct {
	ApplicationProtocol string
	BytesReceived       int64
	BytesSent           int64
	Criticality         *int64
	DestinationDNS      string
	DestinationCountry  string
	DestinationHostID   string
	DestinationIP       string
	DestinationPort     int64
	End                 string
	Errors              []string
	FalsePositive       *bool
	Flags               []string
	HasFiles            bool
	ID                  string
	Protocol            string
	ReportCategory      string
	ReportColor         string
	ReportID            string
	ReportType          string
	ReportWhere         string
	SourceDNS           string
	SourceCountry       string
	SourceHostID        string
	SourceIP            string
	SourcePort          int64
	StoreTag            string
	Start               string
	State               []string
}

func (row *sessionListRow) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 29 {
		return fmt.Errorf("PT NAD session row has %d columns, want 29", len(values))
	}
	decoders := []func(json.RawMessage) error{
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.ApplicationProtocol) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.BytesReceived) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.BytesSent) },
		func(raw json.RawMessage) error { return decodeOptional(raw, &row.Criticality) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.DestinationDNS) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.DestinationCountry) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.DestinationHostID) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.DestinationIP) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.DestinationPort) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.End) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.Errors) },
		func(raw json.RawMessage) error { return decodeOptional(raw, &row.FalsePositive) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.Flags) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.HasFiles) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.ID) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Protocol) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.ReportCategory) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.ReportColor) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.ReportID) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.ReportType) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.ReportWhere) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.SourceDNS) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.SourceCountry) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.SourceHostID) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.SourceIP) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.SourcePort) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.StoreTag) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Start) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.State) },
	}
	for index, decode := range decoders {
		if err := decode(values[index]); err != nil {
			return fmt.Errorf("decode PT NAD session column %d: %w", index, err)
		}
	}
	return nil
}

type attackListResponse struct {
	Took   int64           `json:"took"`
	Total  int64           `json:"total"`
	Result []attackListRow `json:"result"`
}

type attackListRow struct {
	AttackerCountry string
	AttackerHostID  string
	AttackerIP      string
	Class           string
	FalsePositive   *bool
	ID              string
	Message         string
	Priority        *int64
	Revision        int64
	SID             int64
	Timestamp       string
	VictimCountry   string
	VictimHostID    string
	VictimIP        string
	Flows           []attackFlowRow
}

func (row *attackListRow) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 16 {
		return fmt.Errorf("PT NAD attack row has %d columns, want 16", len(values))
	}
	decoders := []func(json.RawMessage) error{
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.AttackerCountry) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.AttackerHostID) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.AttackerIP) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Class) },
		func(raw json.RawMessage) error { return decodeOptional(raw, &row.FalsePositive) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.ID) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Message) },
		func(raw json.RawMessage) error { return decodeOptional(raw, &row.Priority) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Revision) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.SID) },
		func(json.RawMessage) error { return nil }, // success.affected is not a stable scalar.
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Timestamp) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.VictimCountry) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.VictimHostID) },
		func(raw json.RawMessage) error { return decodeNullableString(raw, &row.VictimIP) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.Flows) },
	}
	for index, decode := range decoders {
		if err := decode(values[index]); err != nil {
			return fmt.Errorf("decode PT NAD attack column %d: %w", index, err)
		}
	}
	return nil
}

type attackFlowRow struct {
	ApplicationProtocol string
	DestinationIP       string
	DestinationPort     int64
	End                 string
	Flags               []string
	HasFiles            bool
	ID                  string
	SourceIP            string
	SourcePort          int64
	Start               string
	State               []string
}

func (row *attackFlowRow) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 16 {
		return fmt.Errorf("PT NAD nested flow row has %d columns, want 16", len(values))
	}
	decoders := []func(json.RawMessage) error{
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.ApplicationProtocol) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.DestinationIP) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.DestinationPort) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.End) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.Flags) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.HasFiles) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.ID) },
		func(json.RawMessage) error { return nil },
		func(json.RawMessage) error { return nil },
		func(json.RawMessage) error { return nil },
		func(json.RawMessage) error { return nil },
		func(json.RawMessage) error { return nil },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.SourceIP) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.SourcePort) },
		func(raw json.RawMessage) error { return decodeRequired(raw, &row.Start) },
		func(raw json.RawMessage) error { return decodeNullableSlice(raw, &row.State) },
	}
	for index, decode := range decoders {
		if err := decode(values[index]); err != nil {
			return fmt.Errorf("decode PT NAD nested flow column %d: %w", index, err)
		}
	}
	return nil
}

func decodeRequired[T any](raw json.RawMessage, target *T) error {
	if string(raw) == "null" {
		return fmt.Errorf("required value is null")
	}
	return json.Unmarshal(raw, target)
}

func decodeOptional[T any](raw json.RawMessage, target **T) error {
	if string(raw) == "null" {
		*target = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}

func decodeNullableString(raw json.RawMessage, target *string) error {
	if string(raw) == "null" {
		*target = ""
		return nil
	}
	return json.Unmarshal(raw, target)
}

func decodeNullableSlice[T any](raw json.RawMessage, target *[]T) error {
	if string(raw) == "null" {
		*target = nil
		return nil
	}
	return json.Unmarshal(raw, target)
}

type flowDetail struct {
	Index                string              `json:"_index"`
	ID                   string              `json:"id"`
	Start                string              `json:"start"`
	End                  string              `json:"end"`
	Duration             float64             `json:"duration"`
	ApplicationProtocol  string              `json:"app_proto"`
	ApplicationProtocols []string            `json:"app_protos"`
	Protocol             string              `json:"proto"`
	Source               endpointDTO         `json:"src"`
	Destination          endpointDTO         `json:"dst"`
	Bytes                countersDTO         `json:"bytes"`
	Packets              countersDTO         `json:"pkts"`
	State                []string            `json:"state"`
	Errors               []string            `json:"errors"`
	TCP                  tcpDTO              `json:"tcp"`
	HasFiles             bool                `json:"has_files"`
	FalsePositive        *bool               `json:"false_positive"`
	Criticality          *int64              `json:"criticality"`
	StoreTag             string              `json:"stag"`
	Banners              bannerDTO           `json:"banner"`
	OperatingSystems     operatingSystemsDTO `json:"os"`
	Alerts               []alertDetail       `json:"alert"`
	Files                []fileDetail        `json:"files"`
	Credentials          []credentialDTO     `json:"credentials"`
	SSH                  []sshDTO            `json:"ssh"`
	HTTP                 []httpDTO           `json:"http"`
	SMB                  []smbDTO            `json:"smb"`
	DCERPC               []dcerpcDTO         `json:"dcerpc"`
	NTLM                 []ntlmDTO           `json:"ntlm"`
}

type endpointDTO struct {
	DNS     string   `json:"dns"`
	Groups  []string `json:"groups"`
	HostID  string   `json:"host_id"`
	IP      string   `json:"ip"`
	MAC     string   `json:"mac"`
	Name    string   `json:"name"`
	Port    int64    `json:"port"`
	Country string   `json:"country"`
	Geo     struct {
		Country string `json:"country"`
	} `json:"geo"`
}

type countersDTO struct {
	Received int64 `json:"recv"`
	Sent     int64 `json:"sent"`
	Total    int64 `json:"total"`
}

type tcpDTO struct {
	Flags       []string `json:"flags"`
	FlagsClient []string `json:"flags_tc"`
	FlagsServer []string `json:"flags_ts"`
}

type bannerDTO struct {
	Client []string `json:"client"`
	Server []string `json:"server"`
}

type operatingSystemsDTO struct {
	Client []string `json:"client"`
	Server []string `json:"server"`
}

type alertDetail struct {
	ID            string      `json:"id"`
	Parent        string      `json:"parent"`
	Timestamp     string      `json:"ts"`
	Message       string      `json:"msg"`
	Class         string      `json:"cls"`
	GID           int64       `json:"gid"`
	SID           int64       `json:"sid"`
	Revision      int64       `json:"rev"`
	Priority      *int64      `json:"pr"`
	FalsePositive *bool       `json:"false_positive"`
	Attacker      endpointDTO `json:"attacker"`
	Victim        endpointDTO `json:"victim"`
	ToServer      bool        `json:"to_server"`
	ToClient      bool        `json:"to_client"`
	MatchType     string      `json:"match_type"`
	ATTACK        []string    `json:"att_ck"`
	Signature     struct {
		Vendor      string `json:"vendor"`
		Description struct {
			Name           string `json:"name"`
			AttackTarget   string `json:"attack_target"`
			Description    string `json:"description"`
			Recommendation string `json:"recommendation"`
			AttackFlag     *bool  `json:"attack_flag"`
			Disabled       *bool  `json:"disabled"`
		} `json:"description"`
	} `json:"signature"`
}

type fileDetail struct {
	ID       string `json:"id"`
	Parent   string `json:"parent"`
	FileID   int64  `json:"file_id"`
	TxID     *int64 `json:"tx_id"`
	Filename string `json:"filename"`
	Filepath string `json:"filepath"`
	Magic    string `json:"magic"`
	MD5      string `json:"md5"`
	MIME     string `json:"mime"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	State    string `json:"state"`
	ToServer bool   `json:"to_server"`
	ToClient bool   `json:"to_client"`
}

type credentialDTO struct {
	Login string `json:"login"`
	Valid *bool  `json:"valid"`
}

type transactionDTO struct {
	ID        string `json:"id"`
	Parent    string `json:"parent"`
	TxID      int64  `json:"tx_id"`
	Timestamp string `json:"tx_time"`
}

type sshDTO struct {
	transactionDTO
	Authentication      string      `json:"auth"`
	FailedPasswordCount *int64      `json:"auth_pwd_failed"`
	KeyPressed          *bool       `json:"keypressed"`
	Compression         []string    `json:"compression"`
	Encryption          []string    `json:"encryption"`
	Client              softwareDTO `json:"cli"`
	Server              softwareDTO `json:"srv"`
}

type softwareDTO struct {
	ProtocolVersion string `json:"proto_ver"`
	SoftwareVersion string `json:"soft_ver"`
}

type httpDTO struct {
	transactionDTO
	Request struct {
		Method        string `json:"method"`
		URL           string `json:"url"`
		NormalizedURL string `json:"url_normalized"`
		Protocol      string `json:"proto"`
		EntityLength  int64  `json:"entity_len"`
		Host          string `json:"host"`
		ContentType   string `json:"content-type"`
	} `json:"rqs"`
	Response struct {
		Code         int64  `json:"code"`
		Status       string `json:"status"`
		Protocol     string `json:"proto"`
		EntityLength int64  `json:"entity_len"`
		Server       string `json:"server"`
		ContentType  string `json:"content-type"`
	} `json:"rsp"`
}

type smbDTO struct {
	transactionDTO
	Request  smbMessageDTO `json:"rqs"`
	Response smbMessageDTO `json:"rsp"`
}

type smbMessageDTO struct {
	Command string   `json:"command"`
	Status  string   `json:"status"`
	Flags   []string `json:"flags"`
	Create  struct {
		Filename string `json:"filename"`
		Action   string `json:"action"`
	} `json:"create"`
	TreeConnect struct {
		TreePath  string `json:"tree_path"`
		ShareType string `json:"share_type"`
	} `json:"tree_connect"`
}

type dcerpcDTO struct {
	transactionDTO
	Request  dcerpcMessageDTO `json:"rqs"`
	Response dcerpcMessageDTO `json:"rsp"`
}

type dcerpcMessageDTO struct {
	PacketType string `json:"ptype"`
	Interface  string `json:"interface"`
	Operation  struct {
		Name string `json:"name"`
	} `json:"operation"`
	Auth struct {
		Type  string `json:"type"`
		Level string `json:"level"`
	} `json:"auth"`
}

// ntlmDTO intentionally has no challenge, proof, response, MIC, session key,
// or channel-binding fields. encoding/json discards those response values.
type ntlmDTO struct {
	transactionDTO
	Request struct {
		MessageType string `json:"msg_type"`
		UserName    string `json:"user_name"`
		HostName    string `json:"host_name"`
		TargetName  string `json:"target_name"`
		OSVersion   string `json:"os_version"`
		OSBuild     int64  `json:"os_build"`
	} `json:"rqs"`
	Response struct {
		MessageType string `json:"msg_type"`
		TargetName  string `json:"target_name"`
		OSVersion   string `json:"os_version"`
		OSBuild     int64  `json:"os_build"`
		TargetInfo  struct {
			ServerName    string `json:"server_name"`
			DomainName    string `json:"domain_name"`
			DNSHostName   string `json:"dns_host_name"`
			DNSDomainName string `json:"dns_domain_name"`
		} `json:"target_info"`
	} `json:"rsp"`
}

type storeDetail struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	LastImport  string `json:"last_import"`
	Created     string `json:"created"`
	Modified    string `json:"modified"`
	FilesCount  int64  `json:"files_count"`
	Volume      int64  `json:"volume"`
	IsLive      bool   `json:"is_live"`
}
