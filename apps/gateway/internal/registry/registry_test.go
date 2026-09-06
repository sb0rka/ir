package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type stubEvents struct{}

func (stubEvents) SearchEvents(context.Context, capability.Access, capability.SearchEventsRequest) (capability.EventPage, error) {
	return capability.EventPage{}, nil
}
func (stubEvents) ResolveContext(context.Context, capability.Access, capability.ResolveContextRequest) (capability.ContextPage, error) {
	return capability.ContextPage{}, nil
}

func TestNewRejectsProjectSecretWithoutCredential(t *testing.T) {
	_, err := New(Provider{
		Source: domain.Source{Code: "pt-maxpatrol-siem", Capabilities: []domain.Capability{domain.CapabilityEvents}},
		Events: stubEvents{},
	})
	if err == nil || !strings.Contains(err.Error(), "credential secret") {
		t.Fatalf("expected credential secret error, got %v", err)
	}
}

func TestNewAcceptsProcessModeWithoutCredential(t *testing.T) {
	reg, err := New(Provider{
		Source:         domain.Source{Code: "wazuh", Capabilities: []domain.Capability{domain.CapabilityEvents}},
		CredentialMode: CredentialModeProcess,
		Events:         stubEvents{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := reg.Provider("wazuh"); !ok {
		t.Fatal("provider missing")
	}
}

func TestNewRejectsUnknownCredentialMode(t *testing.T) {
	_, err := New(Provider{
		Source:         domain.Source{Code: "wazuh", Capabilities: []domain.Capability{domain.CapabilityEvents}},
		CredentialMode: "magic",
		Events:         stubEvents{},
	})
	if err == nil || !strings.Contains(err.Error(), "credential mode") {
		t.Fatalf("expected credential mode error, got %v", err)
	}
}
