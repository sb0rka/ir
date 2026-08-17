package maxpatrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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
	if len(provider.Source.Capabilities) != 1 || provider.Source.Capabilities[0] != domain.CapabilityEvents || provider.EntityLookup != nil {
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

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
