package psql

import (
	"testing"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

func TestShouldPromoteEventToGraph(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		direct    bool
		want      bool
	}{
		{name: "session", eventType: "network.session", want: true},
		{name: "detection", eventType: "network.detection", want: true},
		{name: "derived HTTP", eventType: "network.http", want: false},
		{name: "derived file", eventType: "network.file", want: false},
		{name: "derived authentication", eventType: "network.authentication", want: false},
		{name: "direct HTTP", eventType: "network.http", direct: true, want: true},
		{name: "SIEM stays compatible", eventType: "siem_event", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldPromoteEventToGraph(model.GatewayEvent{EventType: tt.eventType, Direct: tt.direct})
			if got != tt.want {
				t.Fatalf("shouldPromoteEventToGraph() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphEntitySnapshotIDs(t *testing.T) {
	selection := model.GatewaySelection{
		Entities: []model.GatewayEntity{
			{SnapshotID: "device"},
			{SnapshotID: "ip"},
			{SnapshotID: "domain"},
			{SnapshotID: "file-hash"},
			{SnapshotID: "direct-entity", Direct: true},
		},
		Events: []model.GatewayEvent{
			{
				SnapshotID: "session",
				EventType:  "network.session",
				Entities: []model.GatewayEventEntity{
					{SnapshotID: "device"},
					{SnapshotID: "ip"},
				},
			},
			{
				SnapshotID: "http",
				EventType:  "network.http",
				Entities:   []model.GatewayEventEntity{{SnapshotID: "domain"}},
			},
			{
				SnapshotID: "file",
				EventType:  "network.file",
				Entities:   []model.GatewayEventEntity{{SnapshotID: "file-hash"}},
			},
		},
	}

	got := graphEntitySnapshotIDs(selection)
	for _, snapshotID := range []string{"device", "ip", "direct-entity"} {
		if _, ok := got[snapshotID]; !ok {
			t.Errorf("expected %q to be promoted", snapshotID)
		}
	}
	for _, snapshotID := range []string{"domain", "file-hash"} {
		if _, ok := got[snapshotID]; ok {
			t.Errorf("expected %q to stay out of the graph projection", snapshotID)
		}
	}
}
