package maxpatrol

import (
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

func TestBuildEventSearchQueryUngroupedUsesRootSentinel(t *testing.T) {
	query, err := buildEventSearchQuery(capability.SearchEventsRequest{
		Filter:  `event_src.host = "dkrylova.plat.form"`,
		Columns: []string{"time", "text"},
		Limit:   100,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query.PDQL, "group(") {
		t.Fatalf("ungrouped search must not add group(): %s", query.PDQL)
	}
	if len(query.GroupValues) != 1 || query.GroupValues[0] == nil || *query.GroupValues[0] != "1" {
		t.Fatalf("root groupValues: %#v", query.GroupValues)
	}
}

func TestBuildEventSearchQueryNullGroupPassesNull(t *testing.T) {
	query, err := buildEventSearchQuery(capability.SearchEventsRequest{
		Filter:      `event_src.host = "vsubbotin.plat.form"`,
		Columns:     []string{"time", "text"},
		GroupBy:     []string{"object.process.chain"},
		GroupValues: []*string{nil},
		Limit:       100,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query.PDQL, "group(key: [object.process.chain], agg: COUNT(*) as Cnt)") {
		t.Fatalf("null group must keep group(): %s", query.PDQL)
	}
	if len(query.GroupValues) != 1 || query.GroupValues[0] != nil {
		t.Fatalf("null groupValues: %#v", query.GroupValues)
	}
}

func TestBuildEventSearchQueryGroupsWhenValueSelected(t *testing.T) {
	create := "create"
	query, err := buildEventSearchQuery(capability.SearchEventsRequest{
		Filter:      `event_src.host = "dkrylova.plat.form"`,
		Columns:     []string{"time", "text"},
		Sort:        []capability.EventSort{{Field: "time", Direction: "asc"}},
		GroupBy:     []string{"action"},
		GroupValues: []*string{&create},
		Limit:       100,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query.PDQL, "group(key: [action], agg: COUNT(*) as Cnt)") {
		t.Fatalf("selected group path must add group(): %s", query.PDQL)
	}
	if len(query.GroupValues) != 1 || query.GroupValues[0] == nil || *query.GroupValues[0] != "create" {
		t.Fatalf("selected groupValues: %#v", query.GroupValues)
	}
}
