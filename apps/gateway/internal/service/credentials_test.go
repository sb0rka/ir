package service

import (
	"context"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type recordingSecrets struct {
	calls int
}

func (resolver *recordingSecrets) Resolve(context.Context, string, string, ...string) (map[string]string, error) {
	resolver.calls++
	return map[string]string{"DEMO_COOKIE": "cookie"}, nil
}

type processEvents struct{}

func (processEvents) SearchEvents(context.Context, capability.Access, capability.SearchEventsRequest) (capability.EventPage, error) {
	return capability.EventPage{Status: "complete"}, nil
}
func (processEvents) ResolveContext(context.Context, capability.Access, capability.ResolveContextRequest) (capability.ContextPage, error) {
	return capability.ContextPage{}, nil
}

func TestLoadCredentialsProcessModeSkipsSecrets(t *testing.T) {
	reg, err := registry.New(registry.Provider{
		Source: domain.Source{
			Code:         "wazuh",
			Capabilities: []domain.Capability{domain.CapabilityEvents},
		},
		CredentialMode: registry.CredentialModeProcess,
		Events:         processEvents{},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	secrets := &recordingSecrets{}
	svc := New(reg, secrets, time.Second, time.Second)
	provider, _ := reg.Provider("wazuh")
	snapshot, err := svc.loadCredentials(context.Background(), ProjectAccess{ProjectID: "aaaaaaaaaa", Bearer: "token"}, provider, false)
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if snapshot.cookie != "" {
		t.Fatalf("cookie = %q", snapshot.cookie)
	}
	if secrets.calls != 0 {
		t.Fatalf("secrets called %d times", secrets.calls)
	}
}

func TestLoadCredentialsProjectSecretStillResolves(t *testing.T) {
	reg, err := registry.New(registry.Provider{
		Source: domain.Source{
			Code:         "pt-maxpatrol-siem",
			Capabilities: []domain.Capability{domain.CapabilityEvents},
		},
		CredentialSecret: "DEMO_COOKIE",
		Events:           processEvents{},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	secrets := &recordingSecrets{}
	svc := New(reg, secrets, time.Second, time.Second)
	provider, _ := reg.Provider("pt-maxpatrol-siem")
	snapshot, err := svc.loadCredentials(context.Background(), ProjectAccess{ProjectID: "aaaaaaaaaa", Bearer: "token"}, provider, false)
	if err != nil {
		t.Fatalf("loadCredentials: %v", err)
	}
	if snapshot.cookie != "cookie" {
		t.Fatalf("cookie = %q", snapshot.cookie)
	}
	if secrets.calls != 1 {
		t.Fatalf("secrets called %d times", secrets.calls)
	}
}
