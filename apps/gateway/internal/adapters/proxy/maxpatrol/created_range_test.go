package maxpatrol

import (
	"context"
	"encoding/json"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIncidentDetectionAndCreationRangesAreIndependent(t *testing.T) {
	detected := TimeRange{From: time.Date(2023, 6, 10, 0, 0, 0, 0, time.UTC), To: time.Date(2023, 6, 11, 0, 0, 0, 0, time.UTC)}
	created := TimeRange{From: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload incidentListPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Filter.DetectedAt.From != vendorTime(detected.From) || payload.Filter.DetectedAt.To != vendorTime(detected.To) {
			t.Error("changed detection range")
		}
		if calls == 1 && (payload.Filter.CreatedAt == nil || payload.Filter.CreatedAt.From != vendorTime(created.From) || payload.Filter.CreatedAt.To != vendorTime(created.To)) {
			t.Error("missing creation range")
		}
		if calls == 2 && payload.Filter.CreatedAt != nil {
			t.Error("legacy request acquired creation filter")
		}
		io.WriteString(w, `{"incidents":[],"totalItems":0}`)
	}))
	defer server.Close()
	cfg := proxy.HTTPClientConfig{BaseURL: server.URL, Timeout: time.Second}
	client, err := NewClient(ClientConfig{SIEM: cfg, Incidents: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []*TimeRange{&created, nil} {
		if _, err := client.SearchIncidents(context.Background(), Access{Cookie: "test"}, IncidentSearchRequest{TimeRange: detected, CreatedAtRange: window, Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	created.To = created.From
	if _, err := client.SearchIncidents(context.Background(), Access{Cookie: "test"}, IncidentSearchRequest{TimeRange: detected, CreatedAtRange: &created, Limit: 1}); err == nil {
		t.Fatal("invalid range accepted")
	}
	if calls != 2 {
		t.Fatal("invalid range reached vendor")
	}
}
