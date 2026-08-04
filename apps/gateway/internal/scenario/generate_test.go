package scenario_test

import (
	"encoding/json"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/fixtures"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

func TestExpandProducesDeterministicDataset(t *testing.T) {
	base, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	options := scenario.GenerateOptions{EventCount: 1_000, EndpointCount: 100, HistoryDays: 30}
	first, err := scenario.Expand(base, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scenario.Expand(base, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != options.EventCount {
		t.Fatalf("events=%d", len(first.Events))
	}
	if _, ok := first.Node("host-synthetic-000100"); !ok {
		t.Fatal("last generated endpoint is missing from the node index")
	}
	severities := map[string]map[string]bool{"MaxPatrol": {}, "PT NAD": {}}
	for _, event := range first.Events {
		if bySource, ok := severities[event.Source]; ok {
			bySource[event.Severity] = true
		}
	}
	for source, values := range severities {
		for _, severity := range []string{"info", "low", "medium", "high", "critical"} {
			if !values[severity] {
				t.Fatalf("source %s has no %s events", source, severity)
			}
		}
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("generated dataset is not deterministic")
	}
}

func TestExpandRejectsInvalidOptions(t *testing.T) {
	base, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	_, err = scenario.Expand(base, scenario.GenerateOptions{EventCount: 10, EndpointCount: 10})
	if err == nil {
		t.Fatal("expected invalid history error")
	}
}
