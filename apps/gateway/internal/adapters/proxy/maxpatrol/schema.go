package maxpatrol

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

// EventRecord is the bounded legacy subset selected by the Gateway.
type EventRecord struct {
	Time            string    `json:"time"`
	UUID            string    `json:"uuid"`
	ID              string    `json:"id"`
	Text            string    `json:"text"`
	Importance      string    `json:"importance,omitempty"`
	EventSourceHost string    `json:"event_src.host,omitempty"`
	EventSourceIP   string    `json:"event_src.ip,omitempty"`
	SourceIP        string    `json:"src.ip,omitempty"`
	DestinationIP   string    `json:"dst.ip,omitempty"`
	DestinationPort int64     `json:"dst.port,omitempty"`
	CorrelationName *string   `json:"correlation_name,omitempty"`
	Meta            EventMeta `json:"_meta,omitempty"`
}

type EventMeta struct {
	ID       string   `json:"id,omitempty"`
	Time     string   `json:"time,omitempty"`
	AssetIDs []string `json:"assetIds,omitempty"`
}

// AccountUserinfo is returned by GET /api/account/userinfo.
type AccountUserinfo struct {
	UserID          string   `json:"userId"`
	UserName        string   `json:"userName"`
	FirstName       *string  `json:"firstName"`
	LastName        *string  `json:"lastName"`
	Roles           []string `json:"roles"`
	PasswordExpired bool     `json:"passwordExpired"`
}
