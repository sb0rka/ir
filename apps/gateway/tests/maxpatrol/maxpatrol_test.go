package maxpatrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/fixtures"
	mockmaxpatrol "github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/mock/scenario"
	maxpatrolapi "github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

var testFetchedAt = time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)

func TestMockUsesMappedVendorPage(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	provider := mockmaxpatrol.NewMock(value)
	if !reflect.DeepEqual(provider.Source.Capabilities, []domain.Capability{domain.CapabilityEvents, domain.CapabilityAccountUserinfo}) ||
		provider.EntityLookup != nil || provider.AccountUserinfo == nil {
		t.Fatalf("provider claims an unverified capability: %#v", provider.Source.Capabilities)
	}
	request := capability.SearchEventsRequest{Limit: 1}
	page, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || len(page.Continuations) != 1 || page.Events[0].Provenance.Source != mockmaxpatrol.SourceCode {
		t.Fatalf("unexpected first page: %#v", page)
	}
	repeated, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(page)
	right, _ := json.Marshal(repeated)
	if !bytes.Equal(left, right) {
		t.Fatal("repeated mock call is not byte-stable")
	}
	request.Cursor = page.Continuations[0]
	next, err := provider.Events.SearchEvents(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Events) != 1 || next.Events[0].Provenance.ExternalID == page.Events[0].Provenance.ExternalID {
		t.Fatalf("cursor did not advance: %#v", next)
	}
}

func TestMockSearchEventsMatchesCanonicalIPFromHostDetails(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	provider := mockmaxpatrol.NewMock(value)

	page, err := provider.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Entities: []domain.EntityRef{{Type: "ip", Value: "192.0.2.62"}},
		Limit:    100,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantEventIDs := []string{"ev-13", "ev-12", "ev-11"}
	if got := sourceEventIDs(page.Events); !reflect.DeepEqual(got, wantEventIDs) {
		t.Fatalf("event IDs=%v want=%v", got, wantEventIDs)
	}
	if !hasEntity(page.Entities, "host", "ws-beta.corp.example") {
		t.Fatalf("ws-beta host is missing from normalized entities: %#v", page.Entities)
	}
}

func TestMockSearchEventsPreservesQueryAndHostMatching(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	provider := mockmaxpatrol.NewMock(value)

	tests := []struct {
		name    string
		request capability.SearchEventsRequest
		want    []string
	}{
		{
			name:    "query",
			request: capability.SearchEventsRequest{Query: "impacket_smbexec", Limit: 100},
			want:    []string{"ev-12"},
		},
		{
			name:    "background rdp rule",
			request: capability.SearchEventsRequest{Query: "multiple_failed_rdp_logon", Limit: 100},
			want:    []string{"ev-noise-1"},
		},
		{
			name:    "background share rule",
			request: capability.SearchEventsRequest{Query: "unusual_admin_share_access", Limit: 100},
			want:    []string{"ev-noise-2"},
		},
		{
			name: "host",
			request: capability.SearchEventsRequest{
				Entities: []domain.EntityRef{{Type: "host", Value: "ws-beta.corp.example"}},
				Limit:    100,
			},
			want: []string{"ev-13", "ev-12", "ev-11"},
		},
		{
			name: "background host is isolated from attack chain",
			request: capability.SearchEventsRequest{
				Entities: []domain.EntityRef{{Type: "host", Value: "srv-jump-eu.ops.internal"}},
				Limit:    100,
			},
			want: []string{"ev-noise-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := provider.Events.SearchEvents(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if got := sourceEventIDs(page.Events); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("event IDs=%v want=%v", got, test.want)
			}
		})
	}
}

func TestMockCorrelationNameComesFromCorrelationNode(t *testing.T) {
	value, err := scenario.Load(fixtures.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	provider := mockmaxpatrol.NewMock(value)

	page, err := provider.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Query: "impacket_smbexec",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("expected one event, got %#v", page.Events)
	}
	name, _ := page.Events[0].Attributes["correlation_name"].(string)
	if name != "impacket_smbexec" {
		t.Fatalf("correlation_name=%q", name)
	}

	noise, err := provider.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Query: "multiple_failed_rdp_logon",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(noise.Events) != 1 {
		t.Fatalf("expected one noise event, got %#v", noise.Events)
	}
	noiseName, _ := noise.Events[0].Attributes["correlation_name"].(string)
	if noiseName != "multiple_failed_rdp_logon" {
		t.Fatalf("noise correlation_name=%q", noiseName)
	}
	if hasEntity(noise.Entities, "host", "ws-alpha.corp.example") || hasEntity(noise.Entities, "ip", "192.0.2.44") {
		t.Fatalf("noise event leaked demo entities: %#v", noise.Entities)
	}
}

func TestMockSearchEventsDoesNotMatchIPExcludedByVendorMapping(t *testing.T) {
	value, err := scenario.Load([]byte(`{
		"nodes": [
			{"id":"host-1","data":{"label":"host-1.example","kind":"host","system":"MaxPatrol","details":{"ip":"192.0.2.1"}}},
			{"id":"host-2","data":{"label":"host-2.example","kind":"host","system":"MaxPatrol","details":{"ip":"192.0.2.2"}}},
			{"id":"host-3","data":{"label":"host-3.example","kind":"host","system":"MaxPatrol","details":{"ip":"192.0.2.3"}}}
		],
		"events": [{
			"id":"event-1",
			"source":"MaxPatrol",
			"event_class":"Lateral Movement",
			"event_ts":"2026-07-23T11:20:00Z",
			"title":"Three hosts",
			"severity":"high",
			"entity_ids":["host-1","host-2","host-3"]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	provider := mockmaxpatrol.NewMock(value)

	page, err := provider.Events.SearchEvents(context.Background(), capability.SearchEventsRequest{
		Entities: []domain.EntityRef{{Type: "ip", Value: "192.0.2.3"}},
		Limit:    100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 0 {
		t.Fatalf("third host IP is not emitted by the vendor mapping but matched: %#v", page.Events)
	}
}

func TestContractFixturesAndMapper(t *testing.T) {
	requestRaw := readFixture(t, "testdata/request.json")
	var request maxpatrolapi.EventsRequest
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	if len(request.Filter.Select) != 7 || request.TimeFrom == 0 {
		t.Fatalf("unexpected request DTO: %#v", request)
	}

	responseRaw := readFixture(t, "testdata/response.json")
	var response maxpatrolapi.EventsResponse
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	page, err := maxpatrolapi.ToEventPage(response, 0, testFetchedAt)
	if err != nil {
		t.Fatalf("map response: %v", err)
	}
	if len(page.Events) != 2 || len(page.Entities) != 2 || !page.HasMore {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Events[0].Provenance.ExternalID != response.Events[0].UUID {
		t.Fatal("external ID was not mapped from uuid")
	}
	if page.Continuations[1] != response.Token+":2" {
		t.Fatalf("unexpected continuation: %q", page.Continuations[1])
	}
	if _, ok := page.Events[0].Attributes["vendor_fields"]; !ok {
		t.Fatal("dynamic selected fields were not preserved")
	}
	first, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("mapper output is not byte-stable")
	}
}

func TestToEventPageRejectsMalformedRecord(t *testing.T) {
	_, err := maxpatrolapi.ToEventPage(maxpatrolapi.EventsResponse{
		Token:      "token",
		TotalCount: 1,
		Events: []maxpatrolapi.EventRecord{{
			Time: "2021-03-16T16:01:05Z",
			ID:   "event",
			Text: "text",
		}},
	}, 0, testFetchedAt)
	if err == nil {
		t.Fatal("expected missing uuid to fail")
	}
}

func sourceEventIDs(events []domain.Event) []string {
	ids := make([]string, len(events))
	for index, event := range events {
		ids[index] = event.Provenance.ExternalID
	}
	return ids
}

func hasEntity(entities []domain.Entity, kind, value string) bool {
	for _, entity := range entities {
		if entity.Type == kind && entity.Value == value {
			return true
		}
	}
	return false
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
