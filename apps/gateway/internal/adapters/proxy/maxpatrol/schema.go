package maxpatrol

import "encoding/json"

// EventsRequest is the used subset of POST /api/events/v2/events in SIEM 8.1.
type EventsRequest struct {
	Filter      EventsFilter `json:"filter"`
	GroupValues []string     `json:"groupValues"`
	TimeFrom    int64        `json:"timeFrom"`
	TimeTo      int64        `json:"timeTo,omitempty"`
}

type EventsFilter struct {
	GroupBy []string  `json:"groupBy"`
	OrderBy []OrderBy `json:"orderBy,omitempty"`
	Select  []string  `json:"select"`
	Top     *int64    `json:"top,omitempty"`
	Where   string    `json:"where,omitempty"`
}

type OrderBy struct {
	Field     string `json:"field"`
	SortOrder string `json:"sortOrder"`
}

// EventsResponse is the documented envelope returned by POST /api/events/v2/events.
type EventsResponse struct {
	Token      string        `json:"token"`
	TotalCount int64         `json:"totalCount"`
	Events     []EventRecord `json:"events"`
}

// EventRecord is the stable subset selected by the Gateway. Fields outside the
// fixed select list remain in Fields because SIEM event schemas are dynamic.
type EventRecord struct {
	Time            string                     `json:"time"`
	UUID            string                     `json:"uuid"`
	ID              string                     `json:"id"`
	Text            string                     `json:"text"`
	Importance      string                     `json:"importance,omitempty"`
	EventSourceHost string                     `json:"event_src.host,omitempty"`
	EventSourceIP   string                     `json:"event_src.ip,omitempty"`
	SourceIP        string                     `json:"src.ip,omitempty"`
	DestinationIP   string                     `json:"dst.ip,omitempty"`
	DestinationPort int64                      `json:"dst.port,omitempty"`
	CorrelationName *string                    `json:"correlation_name,omitempty"`
	Meta            EventMeta                  `json:"_meta,omitempty"`
	Fields          map[string]json.RawMessage `json:"-"`
}

type EventMeta struct {
	ID       string   `json:"id,omitempty"`
	Time     string   `json:"time,omitempty"`
	AssetIDs []string `json:"assetIds,omitempty"`
}

func (record *EventRecord) UnmarshalJSON(data []byte) error {
	type plain EventRecord
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{
		"time", "uuid", "id", "text", "importance", "event_src.host",
		"event_src.ip", "src.ip", "dst.ip", "dst.port", "correlation_name", "_meta",
	} {
		delete(fields, key)
	}
	*record = EventRecord(value)
	record.Fields = fields
	return nil
}

// OAuthTokenResponse is returned by the MaxPatrol OAuth token endpoint.
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}
