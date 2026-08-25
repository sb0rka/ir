package maxpatrol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

func TestBuildEventSearchQuerySupportsCapturedCaseControls(t *testing.T) {
	request := capability.SearchEventsRequest{
		Filter:  `event_src.host = "dkrylova.plat.form" and (object.process.chain contains "splunkd.exe" or subject.process.chain contains "splunkd.exe")`,
		Columns: []string{"time", "object.process.cmdline"},
		Sort: []capability.EventSort{
			{Field: "time", Direction: "asc"},
		},
		GroupBy:     []string{"object.process.chain"},
		GroupValues: []*string{stringPointer("cmd.exe ← splunkd.exe")},
		Limit:       1000,
	}

	query, err := buildEventSearchQuery(request, "uuid != null")
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	want := `filter(event_src.host = "dkrylova.plat.form" and (object.process.chain contains "splunkd.exe" or subject.process.chain contains "splunkd.exe")) | select(time, object.process.cmdline, uuid, text, importance) | sort(time asc) | group(key: [object.process.chain], agg: COUNT(*) as Cnt) | sort(Cnt desc) | limit(1000)`
	if query.PDQL != want {
		t.Fatalf("unexpected PDQL\nwant: %s\n got: %s", want, query.PDQL)
	}
	if len(query.GroupValues) != 1 || query.GroupValues[0] == nil || *query.GroupValues[0] != *request.GroupValues[0] {
		t.Fatalf("unexpected group values: %#v", query.GroupValues)
	}
}

func TestBuildEventSearchQueryKeepsNullGroupValue(t *testing.T) {
	query, err := buildEventSearchQuery(capability.SearchEventsRequest{
		GroupBy:     []string{"object.process.chain"},
		GroupValues: []*string{nil},
		Limit:       10,
	}, "uuid != null")
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	if len(query.GroupValues) != 1 || query.GroupValues[0] != nil {
		t.Fatalf("null group was not retained: %#v", query.GroupValues)
	}
	payload, err := json.Marshal(eventSearchQueryV3Request{GroupValues: query.GroupValues})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	if !strings.Contains(string(payload), `"groupValues":[null]`) {
		t.Fatalf("null group was not encoded for MaxPatrol: %s", payload)
	}
}

func TestDomainEventFromRecordKeepsAllowlistedSelectedFields(t *testing.T) {
	record := safeEventRecord{
		Time:               time.Unix(1, 0),
		UUID:               "10dbf0ee-6c1b-11f1-a062-fb0d762d3dd7",
		Text:               "process event",
		Importance:         "medium",
		ObjectProcessID:    "4321",
		ObjectProcessChain: "cmd.exe ← wmiprvse.exe",
	}
	event, _, _, err := domainEventFromRecord(record, time.Unix(2, 0), "")
	if err != nil {
		t.Fatalf("map event: %v", err)
	}
	if event.Attributes["object.process.id"] != "4321" || event.Attributes["object.process.chain"] != record.ObjectProcessChain {
		t.Fatalf("selected fields were not retained: %#v", event.Attributes)
	}
}

func TestBuildEventSearchQueryCombinesEntityAndSourceFilters(t *testing.T) {
	query, err := buildEventSearchQuery(capability.SearchEventsRequest{
		Filter: `action = "login"`,
		Limit:  20,
	}, `src.ip = "10.125.122.4"`)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	if !strings.HasPrefix(query.PDQL, `filter((src.ip = "10.125.122.4") and (action = "login"))`) {
		t.Fatalf("filters were not combined: %s", query.PDQL)
	}
}

func TestBuildEventSearchQueryRejectsUnsafeOrUnknownInput(t *testing.T) {
	tests := []capability.SearchEventsRequest{
		{Filter: `event_src.host != null | limit(1)`, Limit: 10},
		{Filter: `secret.raw_payload != null`, Limit: 10},
		{Columns: []string{"raw"}, Limit: 10},
	}
	for _, request := range tests {
		if _, err := buildEventSearchQuery(request, "uuid != null"); err == nil {
			t.Fatalf("expected request to be rejected: %#v", request)
		}
	}
}

func stringPointer(value string) *string { return &value }
