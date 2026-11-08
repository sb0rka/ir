package capability

// Query metadata is adapter-owned, and contains only the reviewed vocabulary.
type EventField struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Operators []string `json:"operators"`
	Column    bool     `json:"column"`
	Sort      bool     `json:"sort"`
	Group     bool     `json:"group"`
}
type EventQueryDescription struct {
	Languages    []string     `json:"languages"`
	Fields       []EventField `json:"fields"`
	CommonFilter bool         `json:"common_filter"`
	Bounded      bool         `json:"bounded"`
}
type EventQueryPlanner interface {
	EventQueryDescription() EventQueryDescription
	ValidateEventQuery(SearchEventsRequest, bool) error
}
