package ptnad

import "encoding/json"

// SessionDocument contains the stable subset selected from the supplied PT NAD
// Elasticsearch mapping. It is a storage document, not a claimed REST envelope.
type SessionDocument struct {
	LastTime    json.RawMessage `json:"_ltime"`
	Type        json.RawMessage `json:"_type"`
	Action      string          `json:"action,omitempty"`
	Alert       *Alert          `json:"alert,omitempty"`
	Source      Host            `json:"src"`
	Destination Host            `json:"dst"`
	Bytes       TrafficBytes    `json:"bytes"`
	Duration    float64         `json:"duration,omitempty"`
	Proto       string          `json:"proto,omitempty"`
	Protocol    string          `json:"protocol,omitempty"`
}

type Alert struct {
	Description string `json:"description,omitempty"`
	Level       string `json:"level,omitempty"`
}

type Host struct {
	DNS    string   `json:"dns,omitempty"`
	Groups []string `json:"groups,omitempty"`
	HostID string   `json:"host_id,omitempty"`
	IP     string   `json:"ip,omitempty"`
	MAC    string   `json:"mac,omitempty"`
	Name   string   `json:"name,omitempty"`
	Port   int64    `json:"port,omitempty"`
}

type TrafficBytes struct {
	Received int64 `json:"recv,omitempty"`
	Sent     int64 `json:"sent,omitempty"`
}
