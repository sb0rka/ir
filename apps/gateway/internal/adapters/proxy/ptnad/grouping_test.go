package ptnad

import (
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"slices"
	"testing"
	"time"
)

func TestDeviceRequiresHostIDAndSourceInstance(t *testing.T) {
	endpoint := Endpoint{IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Host: "host", HostID: "host-17"}
	first, ok := endpointDevice(endpoint, "19")
	if !ok {
		t.Fatal("missing anchored device")
	}
	same, _ := endpointDevice(endpoint, "19")
	other, _ := endpointDevice(endpoint, "20")
	if first.Value != same.Value || first.Value == other.Value {
		t.Fatal("device namespace is unstable or unscoped")
	}
	if _, ok := endpointDevice(endpoint, ""); ok {
		t.Fatal("instance-free device")
	}
	endpoint.HostID = ""
	if _, ok := endpointDevice(endpoint, "19"); ok {
		t.Fatal("weak identifiers promoted to device")
	}
	mentions := endpointMentions(endpoint, "src", "19")
	if slices.ContainsFunc(mentions, func(m domain.EntityMention) bool { return m.Type == "device" }) {
		t.Fatal("device from weak ID")
	}
}

func TestSessionAndAttackKeepAtomsAndExplicitDeviceIdentifiers(t *testing.T) {
	at := time.Date(2026, 9, 3, 1, 0, 0, 0, time.UTC)
	source := Endpoint{IP: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff", Host: "host", HostID: "host-17"}
	session := Session{SourceRef: SourceRef{SourceCode: "pt-nad", SourceInstance: "19", RecordType: SessionRecordType, ExternalID: "session-1"}, Source: source, Start: at, End: at.Add(time.Minute), Severity: "low"}
	events, entities, relations := decomposeSession(session)
	if len(events) == 0 {
		t.Fatal("lost session event")
	}
	for _, typ := range []string{"device", "ip", "mac", "host"} {
		if !slices.ContainsFunc(entities, func(e domain.Entity) bool { return e.Type == typ }) {
			t.Fatalf("lost %s atom: %+v", typ, entities)
		}
	}
	count := 0
	for _, r := range relations {
		if r.Type == "has_identifier" {
			count++
			if r.OccurredAt == nil || !r.OccurredAt.Equal(at) {
				t.Fatal("missing observation time")
			}
		}
	}
	if count != 3 {
		t.Fatalf("identifier relationships: %d", count)
	}
	for _, e := range entities {
		if e.Type == "device" && e.Attributes["identity_method"] != "pt-nad-host-id" {
			t.Fatal("missing device provenance")
		}
	}
	attack := Attack{SourceRef: session.SourceRef, Attacker: source, OccurredAt: at, Title: "attack", Severity: "low"}
	if !slices.ContainsFunc(canonicalFinding(attack).Entities, func(m domain.EntityMention) bool { return m.Type == "device" }) {
		t.Fatal("finding lost anchor")
	}
}
