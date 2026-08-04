package config

import (
	"strings"
	"testing"
)

func TestLoadMockDatasetConfiguration(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("MOCK_EVENT_COUNT", "25000")
	t.Setenv("MOCK_ENDPOINT_COUNT", "2500")
	t.Setenv("MOCK_HISTORY_DAYS", "180")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mock.EventCount != 25_000 || cfg.Mock.EndpointCount != 2_500 || cfg.Mock.HistoryDays != 180 {
		t.Fatalf("unexpected mock config: %#v", cfg.Mock)
	}
}

func TestLoadRejectsOversizedMockDataset(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	t.Setenv("MOCK_EVENT_COUNT", "1000001")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MOCK_EVENT_COUNT") {
		t.Fatalf("got %v", err)
	}
}
