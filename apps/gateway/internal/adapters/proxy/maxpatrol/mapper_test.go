package maxpatrol

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

var testFetchedAt = time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)

func TestContractFixturesAndMapper(t *testing.T) {
	requestRaw := readFixture(t, "testdata/request.json")
	var request EventsRequest
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	if len(request.Filter.Select) != 7 || request.TimeFrom == 0 {
		t.Fatalf("unexpected request DTO: %#v", request)
	}

	responseRaw := readFixture(t, "testdata/response.json")
	var response EventsResponse
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		t.Fatalf("decode response fixture: %v", err)
	}
	page, err := ToEventPage(response, 0, testFetchedAt)
	if err != nil {
		t.Fatalf("map response: %v", err)
	}
	if len(page.Events) != 2 || len(page.Entities) != 2 || !page.HasMore {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Events[0].Provenance.ExternalID != response.Events[0].UUID {
		t.Fatalf("external ID was not mapped from uuid")
	}
	if page.Continuations[1] != response.Token+":2" {
		t.Fatalf("unexpected continuation: %q", page.Continuations[1])
	}
	if _, ok := page.Events[0].Attributes["vendor_fields"]; !ok {
		t.Fatalf("dynamic selected fields were not preserved")
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
		t.Fatalf("mapper output is not byte-stable")
	}
}

func TestToEventPageRejectsMalformedRecord(t *testing.T) {
	_, err := ToEventPage(EventsResponse{
		Token:      "token",
		TotalCount: 1,
		Events: []EventRecord{{
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
