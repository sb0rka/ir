package ptnad

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/entitymock"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/eventmock"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/scenario"
)

const SourceCode = "pt-nad"

func NewMock(value scenario.Scenario) registry.Provider {
	events := eventmock.New(value, "PT NAD", SourceCode, enrich)
	entities := entitymock.New(value, "PT NAD", SourceCode, nil)
	return registry.Provider{
		Source: domain.Source{
			Code:         SourceCode,
			Name:         "PT NAD",
			Kind:         "ndr",
			Mode:         "mock",
			Status:       "available",
			Capabilities: []domain.Capability{domain.CapabilityEvents, domain.CapabilityEntityLookup},
		},
		Events:       events,
		EntityLookup: entities,
	}
}

func enrich(event scenario.Event, attributes map[string]any) {
	class := strings.ToLower(event.EventClass)
	proto, appProto, port := "TCP", "http", 80
	switch {
	case strings.Contains(class, "dns"):
		proto, appProto, port = "UDP", "dns", 53
	case strings.Contains(class, "lateral"), strings.Contains(class, "smb"):
		appProto, port = "smb", 445
	case strings.Contains(class, "scan"):
		appProto, port = "unknown", 22
	}

	srcIP, dstIP := "10.125.10.44", "10.125.11.90"
	sent, received := 1420, 317440
	if strings.HasPrefix(event.ID, "mock-nad-") {
		value := eventHash(event.ID)
		srcIP = "10.20." + strconv.FormatUint(uint64(value/250%250), 10) + "." + strconv.FormatUint(uint64(value%250+1), 10)
		dstIP = "198.51.100." + strconv.FormatUint(uint64(value%250+1), 10)
		port = []int{port, 443, 8080, 8443}[value%4]
		sent = 512 + int(value%65536)
		received = 1024 + int(value*17%524288)
	}
	attributes["proto"] = proto
	attributes["app_proto"] = appProto
	attributes["src"] = map[string]any{"ip": srcIP}
	attributes["dst"] = map[string]any{"ip": dstIP, "port": port}
	attributes["bytes"] = map[string]any{"sent": sent, "received": received, "total": sent + received}
	attributes["alert"] = map[string]any{"description": event.Title, "level": strings.ToLower(event.Severity)}
	if strings.Contains(class, "download") || strings.Contains(class, "web shell") {
		attributes["http"] = map[string]any{"method": "GET", "status_code": 200}
		attributes["files"] = map[string]any{"extracted": true}
	}
}

func eventHash(value string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return hash.Sum32()
}
