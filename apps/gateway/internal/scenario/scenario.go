package scenario

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type Scenario struct {
	Nodes  []Node  `json:"nodes"`
	Edges  []Edge  `json:"edges"`
	Events []Event `json:"events"`
}

type Node struct {
	ID   string   `json:"id"`
	Data NodeData `json:"data"`
}

type NodeData struct {
	Label     string         `json:"label"`
	Kind      string         `json:"kind"`
	System    string         `json:"system"`
	Severity  string         `json:"severity"`
	EventID   string         `json:"eventId"`
	IsIOC     bool           `json:"isIoc"`
	SourceURL string         `json:"sourceUrl"`
	Details   map[string]any `json:"details"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

type Event struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	EventClass string   `json:"event_class"`
	EventTS    string   `json:"event_ts"`
	Title      string   `json:"title"`
	Severity   string   `json:"severity"`
	EntityIDs  []string `json:"entity_ids"`
}

func Load(raw []byte) (Scenario, error) {
	var value Scenario
	if err := json.Unmarshal(raw, &value); err != nil {
		return Scenario{}, fmt.Errorf("decode mock scenario: %w", err)
	}
	if len(value.Nodes) == 0 || len(value.Events) == 0 {
		return Scenario{}, fmt.Errorf("mock scenario is empty")
	}
	return value, nil
}

func (value Scenario) Node(id string) (Node, bool) {
	for _, node := range value.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func (value Scenario) NodesForSystem(system string) []Node {
	items := make([]Node, 0)
	for _, node := range value.Nodes {
		if strings.EqualFold(node.Data.System, system) {
			items = append(items, node)
		}
	}
	return items
}

func (value Scenario) EventsForSource(source string) []Event {
	items := make([]Event, 0)
	for _, event := range value.Events {
		if strings.EqualFold(event.Source, source) {
			items = append(items, event)
		}
	}
	return items
}

func (value Scenario) EntityForNode(node Node, source string, fetchedAt time.Time) (domain.Entity, bool) {
	kind, entityValue := entityKindAndValue(node)
	if kind == "" || entityValue == "" {
		return domain.Entity{}, false
	}
	provenance := domain.Provenance{
		Source:     source,
		ExternalID: node.ID,
		SourceURL:  node.Data.SourceURL,
		FetchedAt:  fetchedAt,
	}
	entity := domain.NewEntity(kind, entityValue, provenance)
	entity.Attributes = copyMap(node.Data.Details)
	entity.Attributes["label"] = node.Data.Label
	entity.Attributes["system"] = node.Data.System
	if node.Data.IsIOC {
		entity.Attributes["is_ioc"] = true
	}
	return entity, true
}

func ParseTime(raw string) time.Time {
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return value.UTC()
}

func entityKindAndValue(node Node) (string, string) {
	label := strings.TrimSpace(node.Data.Label)
	switch strings.ToLower(node.Data.Kind) {
	case "host":
		return "host", strings.TrimSuffix(label, "...")
	case "process":
		return "process", label
	case "file":
		return "file", label
	case "external-host":
		host, _, err := net.SplitHostPort(label)
		if err == nil {
			return "ip", host
		}
		return "host", label
	case "persistence":
		return "persistence", label
	default:
		return "", ""
	}
}

func copyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+2)
	for key, value := range input {
		output[key] = value
	}
	return output
}
