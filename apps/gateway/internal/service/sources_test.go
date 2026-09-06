package service

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type probeProvider struct {
	calls  atomic.Int32
	err    error
	status string
}

func (provider *probeProvider) Probe(context.Context, capability.Access) (string, error) {
	provider.calls.Add(1)
	if provider.err != nil {
		return "offline", provider.err
	}
	if provider.status == "" {
		return "online", nil
	}
	return provider.status, nil
}

func TestListSourcesCachesOfflineStatus(t *testing.T) {
	prober := &probeProvider{err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	reg, err := registry.New(registry.Provider{
		Source: domain.Source{
			Code:         "pt-nad",
			Name:         "PT NAD",
			Capabilities: []domain.Capability{domain.CapabilityEvents},
		},
		CredentialSecret: "DEMO_COOKIE",
		Events:           processEvents{},
		Prober:           prober,
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	svc := New(reg, &recordingSecrets{}, time.Second, time.Second)
	access := ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}

	first := svc.ListSources(context.Background(), access, []string{"pt-nad"}, false)
	if len(first) != 1 || first[0].Status != "offline" {
		t.Fatalf("first probe: %+v", first)
	}
	second := svc.ListSources(context.Background(), access, []string{"pt-nad"}, false)
	if len(second) != 1 || second[0].Status != "offline" {
		t.Fatalf("cached probe: %+v", second)
	}
	if prober.calls.Load() != 1 {
		t.Fatalf("expected one probe, got %d", prober.calls.Load())
	}
}

func TestCallProviderDoesNotReloadCredentialsOnTimeout(t *testing.T) {
	prober := &probeProvider{err: context.DeadlineExceeded}
	reg, err := registry.New(registry.Provider{
		Source: domain.Source{
			Code:         "pt-maxpatrol-siem",
			Capabilities: []domain.Capability{domain.CapabilityEvents},
		},
		CredentialSecret: "DEMO_COOKIE",
		Events:           processEvents{},
		Prober:           prober,
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	secrets := &recordingSecrets{}
	svc := New(reg, secrets, time.Second, time.Second)
	provider, _ := reg.Provider("pt-maxpatrol-siem")
	err = svc.callProvider(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}, provider, func(ctx context.Context, access capability.Access) error {
		_, innerErr := prober.Probe(ctx, access)
		return innerErr
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if secrets.calls != 1 {
		t.Fatalf("secrets called %d times, want 1 (no timeout reload)", secrets.calls)
	}
	if prober.calls.Load() != 1 {
		t.Fatalf("provider called %d times, want 1", prober.calls.Load())
	}
}
