package main

import (
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/config"
)

func TestRegistryUsesConfiguredBackendsWithoutProjectMapping(t *testing.T) {
	empty, err := buildRegistry(config.Config{})
	if err != nil || len(empty.Sources()) != 0 {
		t.Fatal("empty configuration did not produce empty registry", err)
	}
	configured, err := buildRegistry(config.Config{Sources: map[string]config.SourceConfig{
		config.PTMaxPatrolSIEM: {BaseURL: "https://siem.example", IncidentsBaseURL: "https://incidents.example", Timeout: time.Second},
		config.PTNAD:           {BaseURL: "https://nad.example", StoreIDs: []string{"19"}, Timeout: time.Second},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(configured.Sources()) != 2 {
		t.Fatal("configured backends were not registered")
	}
}
