package wazuh

import "encoding/json"

type searchRequest struct {
	Size            int              `json:"size"`
	TrackTotalHits  bool             `json:"track_total_hits"`
	Timeout         string           `json:"timeout,omitempty"`
	Query           map[string]any   `json:"query"`
	Sort            []map[string]any `json:"sort"`
	SearchAfter     []any            `json:"search_after,omitempty"`
	Source          sourceFilter     `json:"_source"`
	Aggregations    map[string]any   `json:"aggs,omitempty"`
}

type sourceFilter struct {
	Includes []string `json:"includes"`
}

type searchResponse struct {
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Failed int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total hitsTotal   `json:"total"`
		Hits  []searchHit `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]json.RawMessage `json:"aggregations"`
}

type hitsTotal struct {
	Value    int64  `json:"value"`
	Relation string `json:"relation"`
}

type searchHit struct {
	Index  string      `json:"_index"`
	ID     string      `json:"_id"`
	Sort   []any       `json:"sort"`
	Source alertSource `json:"_source"`
}

type getDocumentResponse struct {
	Index  string      `json:"_index"`
	ID     string      `json:"_id"`
	Found  bool        `json:"found"`
	Source alertSource `json:"_source"`
}

type clusterHealthResponse struct {
	Status string `json:"status"`
}

// alertSource is the bounded subset of a Wazuh alert used by the mapper.
type alertSource struct {
	AtTimestamp string         `json:"@timestamp"`
	Timestamp   string         `json:"timestamp"`
	ID          string         `json:"id"`
	Location    string         `json:"location"`
	Agent       alertAgent     `json:"agent"`
	Manager     alertManager   `json:"manager"`
	Rule        alertRule      `json:"rule"`
	Decoder     alertDecoder   `json:"decoder"`
	Data        alertData      `json:"data"`
	Syscheck    alertSyscheck  `json:"syscheck"`
	GeoLocation alertGeo       `json:"GeoLocation"`
}

type alertAgent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type alertManager struct {
	Name string `json:"name"`
}

type alertRule struct {
	ID          string      `json:"id"`
	Level       int         `json:"level"`
	Description string      `json:"description"`
	Groups      []string    `json:"groups"`
	FiredTimes  int         `json:"firedtimes"`
	Frequency   int         `json:"frequency"`
	Mitre       alertMitre  `json:"mitre"`
}

type alertMitre struct {
	ID        []string `json:"id"`
	Tactic    []string `json:"tactic"`
	Technique []string `json:"technique"`
}

type alertDecoder struct {
	Name string `json:"name"`
}

type alertData struct {
	SrcIP      string `json:"srcip"`
	DstIP      string `json:"dstip"`
	SrcPort    string `json:"srcport"`
	DstPort    string `json:"dstport"`
	SrcUser    string `json:"srcuser"`
	DstUser    string `json:"dstuser"`
	Protocol   string `json:"protocol"`
	URL        string `json:"url"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	SystemName string `json:"system_name"`
	Title      string `json:"title"`
	File       string `json:"file"`
	Command    string `json:"command"`
	Integration string `json:"integration"`

	Win         alertWinData         `json:"win"`
	AWS         alertAWSData         `json:"aws"`
	Office365   alertOffice365       `json:"office365"`
	GitHub      alertGitHub          `json:"github"`
	GCP         alertGCP             `json:"gcp"`
	VirusTotal  alertVirusTotal      `json:"virustotal"`
	Vulnerability alertVulnerability `json:"vulnerability"`
	Audit       alertAudit           `json:"audit"`
	Docker      alertDocker          `json:"docker"`
	Osquery     alertOsquery         `json:"osquery"`
	MSGraph     alertMSGraph         `json:"ms-graph"`
}

type alertWinData struct {
	EventData alertWinEventData `json:"eventdata"`
	System    alertWinSystem    `json:"system"`
}

type alertWinEventData struct {
	TargetUserName string `json:"targetUserName"`
	IPAddress      string `json:"ipAddress"`
	LogonType      string `json:"logonType"`
	ProcessID      string `json:"processId"`
}

type alertWinSystem struct {
	Computer  string `json:"computer"`
	EventID   string `json:"eventID"`
	Channel   string `json:"channel"`
}

type alertAWSData struct {
	Source    string `json:"source"`
	AccountID string `json:"accountId"`
	Region    string `json:"region"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Service   struct {
		Action struct {
			ActionType              string `json:"actionType"`
			NetworkConnectionAction struct {
				ConnectionDirection string `json:"connectionDirection"`
				LocalIPDetails      struct {
					IPAddressV4 string `json:"ipAddressV4"`
				} `json:"localIpDetails"`
				RemoteIPDetails struct {
					IPAddressV4 string `json:"ipAddressV4"`
				} `json:"remoteIpDetails"`
			} `json:"networkConnectionAction"`
			AWSAPICallAction struct {
				RemoteIPDetails struct {
					IPAddressV4 string `json:"ipAddressV4"`
				} `json:"remoteIpDetails"`
			} `json:"awsApiCallAction"`
		} `json:"action"`
	} `json:"service"`
}

type alertOffice365 struct {
	UserID     string `json:"UserId"`
	ClientIP   string `json:"ClientIP"`
	Operation  string `json:"Operation"`
	Workload   string `json:"Workload"`
}

type alertGitHub struct {
	Actor  string `json:"actor"`
	User   string `json:"user"`
	Org    string `json:"org"`
	Repo   string `json:"repo"`
	Action string `json:"action"`
}

type alertGCP struct {
	JSONPayload struct {
		SourceIP       string `json:"sourceIP"`
		VMInstanceName string `json:"vmInstanceName"`
		QueryName      string `json:"queryName"`
	} `json:"jsonPayload"`
	Resource struct {
		Labels struct {
			ProjectID string `json:"project_id"`
		} `json:"labels"`
	} `json:"resource"`
}

type alertVirusTotal struct {
	Positives string `json:"positives"`
	Source    struct {
		MD5  string `json:"md5"`
		SHA1 string `json:"sha1"`
		File string `json:"file"`
	} `json:"source"`
}

type alertVulnerability struct {
	CVE      string `json:"cve"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Title    string `json:"title"`
	Package  struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"package"`
	CVSS struct {
		CVSS3 struct {
			BaseScore float64 `json:"base_score"`
		} `json:"cvss3"`
	} `json:"cvss"`
}

type alertAudit struct {
	Command string `json:"command"`
	Exe     string `json:"exe"`
	Success string `json:"success"`
	Type    string `json:"type"`
	File    struct {
		Name string `json:"name"`
	} `json:"file"`
}

type alertDocker struct {
	Action string `json:"Action"`
	Type   string `json:"Type"`
}

type alertOsquery struct {
	Name   string `json:"name"`
	Action string `json:"action"`
	Pack   string `json:"pack"`
}

type alertMSGraph struct {
	IPAddress string `json:"ipAddress"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
}

type alertSyscheck struct {
	Path         string `json:"path"`
	Event        string `json:"event"`
	MD5After     string `json:"md5_after"`
	SHA1After    string `json:"sha1_after"`
	SHA256After  string `json:"sha256_after"`
}

type alertGeo struct {
	CountryName string `json:"country_name"`
	CityName    string `json:"city_name"`
}

type termsAggregation struct {
	Buckets []termsBucket `json:"buckets"`
}

type termsBucket struct {
	Key      any   `json:"key"`
	DocCount int64 `json:"doc_count"`
}

type filtersAggregation struct {
	Buckets map[string]struct {
		DocCount int64 `json:"doc_count"`
	} `json:"buckets"`
}

type compositeAggregation struct {
	Buckets []struct {
		Key       map[string]any `json:"key"`
		DocCount  int64          `json:"doc_count"`
	} `json:"buckets"`
	AfterKey map[string]any `json:"after_key"`
}
