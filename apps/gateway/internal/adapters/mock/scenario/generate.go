package scenario

import (
	"fmt"
	"strings"
	"time"
)

var syntheticAnchor = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

type GenerateOptions struct {
	EventCount    int
	EndpointCount int
	HistoryDays   int
}

func Expand(base Scenario, options GenerateOptions) (Scenario, error) {
	if options.EventCount < 0 || options.EndpointCount < 0 {
		return Scenario{}, fmt.Errorf("mock event and endpoint counts must not be negative")
	}
	if options.HistoryDays <= 0 {
		return Scenario{}, fmt.Errorf("mock history days must be positive")
	}

	value := Scenario{
		Nodes:  append([]Node(nil), base.Nodes...),
		Edges:  append([]Edge(nil), base.Edges...),
		Events: append([]Event(nil), base.Events...),
	}

	hostIDs := maxPatrolHostIDs(value.Nodes)
	targetEndpoints := max(options.EndpointCount, len(hostIDs))
	for len(hostIDs) < targetEndpoints {
		serial := len(hostIDs) + 1
		node := syntheticHost(serial)
		value.Nodes = append(value.Nodes, node)
		hostIDs = append(hostIDs, node.ID)
	}
	maxPatrolEvents := len(value.EventsForSource("MaxPatrol"))
	targetEvents := max(options.EventCount, maxPatrolEvents)
	for maxPatrolEvents < targetEvents {
		serial := len(value.Events) + 1
		value.Events = append(value.Events, syntheticEvent(serial, hostIDs, options.HistoryDays))
		maxPatrolEvents++
	}
	value.reindex()
	return value, nil
}

func maxPatrolHostIDs(nodes []Node) []string {
	result := make([]string, 0)
	for _, node := range nodes {
		if strings.EqualFold(node.Data.System, "MaxPatrol") && strings.EqualFold(node.Data.Kind, "host") {
			result = append(result, node.ID)
		}
	}
	return result
}

func syntheticHost(serial int) Node {
	hostname := fmt.Sprintf("ws-%06d.corp.example", serial)
	status := "online"
	if serial%13 == 0 {
		status = "offline"
	}
	departments := []string{"engineering", "finance", "operations", "sales", "security", "support"}
	operatingSystems := []string{"Windows 11", "Windows Server 2022", "Ubuntu 24.04", "RED OS 8"}
	sites := []string{"moscow", "spb", "kazan", "novosibirsk"}
	criticalities := []string{"low", "medium", "high", "critical"}
	return Node{
		ID: fmt.Sprintf("host-synthetic-%06d", serial),
		Data: NodeData{
			Label:     hostname,
			Kind:      "host",
			System:    "MaxPatrol",
			Severity:  "info",
			IsIOC:     serial%997 == 0,
			SourceURL: "https://maxpatrol.mock.local/assets/" + hostname,
			Details: map[string]any{
				"ip":          syntheticIP(serial),
				"status":      status,
				"department":  departments[serial%len(departments)],
				"os":          operatingSystems[serial%len(operatingSystems)],
				"site":        sites[serial%len(sites)],
				"criticality": criticalities[serial%len(criticalities)],
			},
		},
	}
}

func syntheticEvent(serial int, hostIDs []string, historyDays int) Event {
	severityWeights := []string{
		"info", "info", "info", "info", "info",
		"low", "low", "low", "low",
		"medium", "medium", "medium", "medium", "medium",
		"high", "high", "high", "high",
		"critical", "critical",
	}
	siemClasses := []string{
		"Authentication Failure", "Process Execution", "Malware Detection", "Persistence",
		"Lateral Movement", "Correlation Alert", "Privilege Escalation", "Account Change",
		"Network Connection", "Policy Violation",
	}
	eventClass := siemClasses[serial%len(siemClasses)]
	severity := severityWeights[(serial*7+serial/4)%len(severityWeights)]
	sourceHost := hostIDs[(serial*37)%len(hostIDs)]
	targetHost := hostIDs[(serial*101+1)%len(hostIDs)]
	periodSeconds := int64(historyDays) * 24 * 60 * 60
	offset := int64(serial*7919) % periodSeconds
	timestamp := syntheticAnchor.Add(-time.Duration(offset) * time.Second)

	return Event{
		ID:         fmt.Sprintf("mock-siem-%08d", serial),
		Source:     "MaxPatrol",
		EventClass: eventClass,
		EventTS:    timestamp.Format(time.RFC3339),
		Title:      fmt.Sprintf("%s on %s targeting %s", eventClass, sourceHost, targetHost),
		Severity:   severity,
		EntityIDs:  []string{sourceHost, targetHost},
	}
}

func syntheticIP(serial int) string {
	return fmt.Sprintf("10.%d.%d.%d", 20+(serial/62500)%10, (serial/250)%250, serial%250+1)
}
