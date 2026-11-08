package config

import (
	"testing"
	"time"
)

func TestEnabledSourcesUseServerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources map[string]SourceConfig
		want    int
		invalid bool
	}{
		{name: "none", sources: map[string]SourceConfig{PTMaxPatrolSIEM: {Timeout: time.Second}, PTNAD: {Timeout: time.Second}}},
		{name: "SIEM", sources: map[string]SourceConfig{PTMaxPatrolSIEM: {BaseURL: "https://siem.example", IncidentsBaseURL: "https://incidents.example", Timeout: time.Second}}, want: 1},
		{name: "NAD", sources: map[string]SourceConfig{PTNAD: {BaseURL: "https://nad.example", StoreIDs: []string{"19"}, Timeout: time.Second}}, want: 1},
		{name: "SIEM missing second URL", sources: map[string]SourceConfig{PTMaxPatrolSIEM: {BaseURL: "https://siem.example", Timeout: time.Second}}, invalid: true},
		{name: "NAD missing stores", sources: map[string]SourceConfig{PTNAD: {BaseURL: "https://nad.example", Timeout: time.Second}}, invalid: true},
		{name: "NAD missing URL", sources: map[string]SourceConfig{PTNAD: {StoreIDs: []string{"19"}, Timeout: time.Second}}, invalid: true},
		{name: "HTTP URL", sources: map[string]SourceConfig{PTNAD: {BaseURL: "http://nad.example", StoreIDs: []string{"19"}, Timeout: time.Second}}, want: 1},
		{name: "URL credentials", sources: map[string]SourceConfig{PTNAD: {BaseURL: "https://user:secret@nad.example", StoreIDs: []string{"19"}, Timeout: time.Second}}, invalid: true},
		{name: "unknown source", sources: map[string]SourceConfig{"unregistered": {BaseURL: "https://unknown.example", Timeout: time.Second}}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := enabledSources(tc.sources)
			if (err != nil) != tc.invalid || err == nil && len(got) != tc.want {
				t.Fatalf("sources=%d err=%v", len(got), err)
			}
		})
	}
}
