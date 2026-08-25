package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type aggregationSecrets struct {
	calls atomic.Int32
}

func (secrets *aggregationSecrets) Resolve(_ context.Context, _, _ string, names ...string) (map[string]string, error) {
	secrets.calls.Add(1)
	values := make(map[string]string, len(names))
	for _, name := range names {
		values[name] = "cookie=value"
	}
	return values, nil
}

type aggregationEventSource struct{}

func (aggregationEventSource) SearchEvents(context.Context, capability.Access, capability.SearchEventsRequest) (capability.EventPage, error) {
	return capability.EventPage{}, nil
}

func (aggregationEventSource) ResolveContext(context.Context, capability.Access, capability.ResolveContextRequest) (capability.ContextPage, error) {
	return capability.ContextPage{}, nil
}

type aggregationProvider struct {
	call  func(capability.AggregateEventsRequest, int32) (capability.EventGroupPage, error)
	calls atomic.Int32
}

func (provider *aggregationProvider) AggregateEvents(_ context.Context, _ capability.Access, request capability.AggregateEventsRequest) (capability.EventGroupPage, error) {
	call := provider.calls.Add(1)
	return provider.call(request, call)
}

func TestAggregateEventsMixedSourcesAndPerSourceLimit(t *testing.T) {
	aggregator := &aggregationProvider{call: func(request capability.AggregateEventsRequest, _ int32) (capability.EventGroupPage, error) {
		if request.Limit != 1 {
			t.Errorf("provider limit = %d", request.Limit)
		}
		first, second := "high", "low"
		return capability.EventGroupPage{Status: "complete", Groups: []domain.EventGroup{
			{SourceCode: "untrusted", Values: []*string{&first}, Count: 3},
			{SourceCode: "untrusted", Values: []*string{&second}, Count: 2},
		}}, nil
	}}
	reg := aggregationRegistry(t,
		aggregationRegistryProvider("a-siem", aggregator),
		aggregationRegistryProvider("z-nad", nil),
	)
	service := New(reg, &aggregationSecrets{}, time.Second, time.Second)
	result, err := service.AggregateEvents(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}, AggregateEventsRequest{
		TimeRange: domain.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)},
		GroupBy:   []string{"importance"},
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("aggregate events: %v", err)
	}
	if len(result.Groups) != 1 || result.Groups[0].SourceCode != "a-siem" || result.Groups[0].Count != 3 {
		t.Fatalf("unexpected groups: %+v", result.Groups)
	}
	if len(result.SourceStates) != 2 || result.SourceStates[0].Source != "a-siem" || result.SourceStates[0].Status != "truncated" || result.SourceStates[1].Source != "z-nad" || result.SourceStates[1].Status != "failed" {
		t.Fatalf("unexpected source states: %+v", result.SourceStates)
	}
	if len(result.SourceErrors) != 1 || result.SourceErrors[0].Code != "unsupported_event_aggregation" || result.SourceErrors[0].Retryable {
		t.Fatalf("unexpected source errors: %+v", result.SourceErrors)
	}
}

func TestAggregateEventsOrdersSourcesAfterConcurrentFanout(t *testing.T) {
	valueA, valueB := "a", "b"
	providerA := &aggregationProvider{call: func(capability.AggregateEventsRequest, int32) (capability.EventGroupPage, error) {
		time.Sleep(10 * time.Millisecond)
		return capability.EventGroupPage{Groups: []domain.EventGroup{{Values: []*string{&valueA}, Count: 1}}}, nil
	}}
	providerB := &aggregationProvider{call: func(capability.AggregateEventsRequest, int32) (capability.EventGroupPage, error) {
		return capability.EventGroupPage{Groups: []domain.EventGroup{{Values: []*string{&valueB}, Count: 2}}}, nil
	}}
	reg := aggregationRegistry(t,
		aggregationRegistryProvider("b-source", providerB),
		aggregationRegistryProvider("a-source", providerA),
	)
	service := New(reg, &aggregationSecrets{}, time.Second, time.Second)
	result, err := service.AggregateEvents(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}, validServiceAggregationRequest())
	if err != nil {
		t.Fatalf("aggregate events: %v", err)
	}
	if len(result.Groups) != 2 || result.Groups[0].SourceCode != "a-source" || result.Groups[1].SourceCode != "b-source" {
		t.Fatalf("groups are not source ordered: %+v", result.Groups)
	}
}

func TestAggregateEventsUnsupportedOnly(t *testing.T) {
	reg := aggregationRegistry(t, aggregationRegistryProvider("pt-nad", nil))
	service := New(reg, &aggregationSecrets{}, time.Second, time.Second)
	_, err := service.AggregateEvents(context.Background(), ProjectAccess{}, validServiceAggregationRequest())
	if !errors.Is(err, domain.ErrUnsupportedCapability) {
		t.Fatalf("expected unsupported capability, got %v", err)
	}
}

func TestAggregateEventsAllSupportingSourcesFail(t *testing.T) {
	provider := &aggregationProvider{call: func(capability.AggregateEventsRequest, int32) (capability.EventGroupPage, error) {
		return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 500}
	}}
	reg := aggregationRegistry(t, aggregationRegistryProvider("siem", provider))
	service := New(reg, &aggregationSecrets{}, time.Second, time.Second)
	_, err := service.AggregateEvents(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}, validServiceAggregationRequest())
	var allSources *AllSourcesError
	if !errors.As(err, &allSources) || len(allSources.Items) != 1 || allSources.Items[0].Code != "provider_error" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestAggregateEventsReloadsCredentialsOnce(t *testing.T) {
	provider := &aggregationProvider{call: func(_ capability.AggregateEventsRequest, call int32) (capability.EventGroupPage, error) {
		if call == 1 {
			return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 401}
		}
		return capability.EventGroupPage{Status: "complete"}, nil
	}}
	secrets := &aggregationSecrets{}
	reg := aggregationRegistry(t, aggregationRegistryProvider("siem", provider))
	service := New(reg, secrets, time.Second, time.Second)
	if _, err := service.AggregateEvents(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "token"}, validServiceAggregationRequest()); err != nil {
		t.Fatalf("aggregate events: %v", err)
	}
	if provider.calls.Load() != 2 || secrets.calls.Load() != 2 {
		t.Fatalf("calls: provider=%d secrets=%d", provider.calls.Load(), secrets.calls.Load())
	}
}

func aggregationRegistry(t *testing.T, providers ...registry.Provider) *registry.Registry {
	t.Helper()
	reg, err := registry.New(providers...)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	return reg
}

func aggregationRegistryProvider(source string, aggregator capability.EventAggregator) registry.Provider {
	return registry.Provider{
		Source:           domain.Source{Code: source, Capabilities: []domain.Capability{domain.CapabilityEvents}},
		CredentialSecret: "credential-" + source,
		Events:           aggregationEventSource{},
		EventAggregation: aggregator,
	}
}

func validServiceAggregationRequest() AggregateEventsRequest {
	return AggregateEventsRequest{
		TimeRange: domain.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)},
		GroupBy:   []string{"importance"},
		Limit:     100,
	}
}
