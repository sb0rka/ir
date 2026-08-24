package ptnad

import (
	"testing"
	"time"
)

func TestDecomposeSessionAggregatesEndpointIdentifiers(t *testing.T) {
	occurredAt := time.Date(2025, 1, 18, 22, 54, 31, 0, time.UTC)
	session := Session{
		SourceRef: SourceRef{
			SourceCode: SourceCode, SourceInstance: "22", RecordType: SessionRecordType, ExternalID: "case-2.5",
		},
		FetchedAt: occurredAt.Add(time.Hour),
		Start:     occurredAt,
		End:       occurredAt.Add(time.Second),
		Source: Endpoint{
			IP: "10.10.187.54", MAC: "00-50-56-B4-0D-3F",
		},
		Destination: Endpoint{
			IP: "10.10.187.241", MAC: "00:50:56:B4:C5:83",
		},
		HTTP: []HTTPHint{{
			Transaction: TransactionRef{ExternalID: "http-1", OccurredAt: occurredAt},
			Method:      "GET", Path: "/Antichat_Shell.php", Host: "10.10.187.241",
		}},
	}

	events, entities, relations := decomposeSession(session)

	wantEntities := map[string]bool{
		"device\x0000:50:56:b4:0d:3f": false,
		"device\x0000:50:56:b4:c5:83": false,
		"ip\x0010.10.187.54":          false,
		"ip\x0010.10.187.241":         false,
		"mac\x0000:50:56:b4:0d:3f":    false,
		"mac\x0000:50:56:b4:c5:83":    false,
	}
	for _, entity := range entities {
		key := entity.Type + "\x00" + entity.Value
		if _, exists := wantEntities[key]; exists {
			wantEntities[key] = true
		}
		if entity.Type == "domain" && entity.Value == "10.10.187.241" {
			t.Fatal("HTTP Host containing an IP literal must remain an IP observable")
		}
	}
	for key, found := range wantEntities {
		if !found {
			t.Errorf("missing entity %q", key)
		}
	}

	hasIdentifier := 0
	for _, relation := range relations {
		if relation.Type == "has_identifier" {
			hasIdentifier++
			if relation.SourceEntity.Type != "device" {
				t.Errorf("has_identifier source type = %q, want device", relation.SourceEntity.Type)
			}
		}
	}
	if hasIdentifier != 4 {
		t.Fatalf("has_identifier relations = %d, want 4", hasIdentifier)
	}

	foundHTTPObject := false
	for _, event := range events {
		if event.Type != "network.http" {
			continue
		}
		for _, entity := range event.Entities {
			if entity.Type == "ip" && entity.Value == "10.10.187.241" {
				for _, role := range entity.Roles {
					foundHTTPObject = foundHTTPObject || role == "object"
				}
			}
		}
	}
	if !foundHTTPObject {
		t.Fatal("HTTP Host IP observable is missing the object role")
	}

	parentID := sourceEventID(session.SourceRef)
	for _, event := range events {
		if event.Type == "network.session" {
			continue
		}
		if event.Attributes["parent_source_event_id"] != parentID {
			t.Errorf("%s parent_source_event_id = %v, want %q", event.Type, event.Attributes["parent_source_event_id"], parentID)
		}
		if event.Attributes["relation_type"] != "subevent_of" {
			t.Errorf("%s relation_type = %v, want subevent_of", event.Type, event.Attributes["relation_type"])
		}
	}
}

func TestCanonicalFindingUsesSuspectedEndpointRoles(t *testing.T) {
	finding := canonicalFinding(Attack{
		SourceRef: SourceRef{SourceCode: SourceCode, SourceInstance: "22", RecordType: AttackRecordType, ExternalID: "attack-1"},
		Attacker:  Endpoint{IP: "10.10.187.241"},
		Victim:    Endpoint{IP: "10.10.187.54"},
	})

	want := map[string]string{
		"10.10.187.241": "suspected_source",
		"10.10.187.54":  "suspected_target",
	}
	for _, entity := range finding.Entities {
		role, ok := want[entity.Value]
		if !ok || entity.Type != "ip" {
			continue
		}
		if len(entity.Roles) != 1 || entity.Roles[0] != role {
			t.Errorf("%s roles = %v, want [%s]", entity.Value, entity.Roles, role)
		}
		delete(want, entity.Value)
	}
	for value, role := range want {
		t.Errorf("missing %s role for %s", role, value)
	}
}
