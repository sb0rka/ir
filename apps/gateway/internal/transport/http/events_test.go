package httptransport

import (
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/api"
)

func TestSearchEventsRequestMapsAdvancedControls(t *testing.T) {
	filter := `event_src.host = "dkrylova.plat.form"`
	columns := []string{"time", "object.process.cmdline"}
	groupBy := []string{"object.process.chain"}
	groupValue := "cmd.exe ← splunkd.exe"
	groupValues := []*string{&groupValue}
	sortRules := []api.EventSort{{Field: "time", Direction: api.Asc}}
	request, err := searchEventsRequest(api.SearchEventsRequest{
		TimeRange: api.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)},
		Filter:    &filter, Columns: &columns, Sort: &sortRules, GroupBy: &groupBy, GroupValues: &groupValues,
	})
	if err != nil {
		t.Fatalf("map request: %v", err)
	}
	if request.Filter != filter || len(request.Columns) != 2 || len(request.Sort) != 1 || request.Sort[0].Direction != "asc" {
		t.Fatalf("advanced controls were not mapped: %#v", request)
	}
	if len(request.GroupBy) != 1 || len(request.GroupValues) != 1 {
		t.Fatalf("group controls were not mapped: %#v", request)
	}
}

func TestSearchEventsRequestRejectsInvalidAdvancedControls(t *testing.T) {
	timeRange := api.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)}
	unsafeFilter := `event_src.host != null | limit(1)`
	if _, err := searchEventsRequest(api.SearchEventsRequest{TimeRange: timeRange, Filter: &unsafeFilter}); err == nil {
		t.Fatal("expected pipeline filter to be rejected")
	}
	groupBy := []string{"action", "importance"}
	groupValue := "login"
	groupValues := []*string{&groupValue}
	if _, err := searchEventsRequest(api.SearchEventsRequest{TimeRange: timeRange, GroupBy: &groupBy, GroupValues: &groupValues}); err == nil {
		t.Fatal("expected misaligned group values to be rejected")
	}
}

func TestSearchEventsRequestAcceptsNullGroup(t *testing.T) {
	groupBy := []string{"object.process.chain"}
	groupValues := []*string{nil}
	request, err := searchEventsRequest(api.SearchEventsRequest{
		TimeRange: api.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)},
		GroupBy:   &groupBy, GroupValues: &groupValues,
	})
	if err != nil {
		t.Fatalf("map null group: %v", err)
	}
	if len(request.GroupValues) != 1 || request.GroupValues[0] != nil {
		t.Fatalf("null group was not mapped: %#v", request.GroupValues)
	}
}
