package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type Provider struct {
	Source           domain.Source
	Events           capability.EventSource
	EntityLookup     capability.EntityLookup
	ArtifactAnalyzer capability.ArtifactAnalyzer
	Endpoints        capability.EndpointSource
	ResponseCatalog  capability.ResponseCatalog
}

type Registry struct {
	providers map[string]Provider
}

func New(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		code := strings.TrimSpace(provider.Source.Code)
		if code == "" {
			return nil, fmt.Errorf("provider code is required")
		}
		if _, exists := registry.providers[code]; exists {
			return nil, fmt.Errorf("provider %q is registered twice", code)
		}
		if err := validateCapabilities(provider); err != nil {
			return nil, fmt.Errorf("provider %q: %w", code, err)
		}
		registry.providers[code] = provider
	}
	return registry, nil
}

func (registry *Registry) Sources() []domain.Source {
	items := make([]domain.Source, 0, len(registry.providers))
	for _, provider := range registry.providers {
		items = append(items, provider.Source)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}

func (registry *Registry) Provider(code string) (Provider, bool) {
	provider, ok := registry.providers[strings.TrimSpace(code)]
	return provider, ok
}

func (registry *Registry) Select(codes []string, capabilityName domain.Capability) ([]Provider, error) {
	if len(codes) == 0 {
		providers := make([]Provider, 0, len(registry.providers))
		for _, provider := range registry.providers {
			if supports(provider.Source, capabilityName) {
				providers = append(providers, provider)
			}
		}
		sort.Slice(providers, func(i, j int) bool { return providers[i].Source.Code < providers[j].Source.Code })
		return providers, nil
	}

	seen := make(map[string]struct{}, len(codes))
	providers := make([]Provider, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		provider, ok := registry.providers[code]
		if !ok {
			return nil, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", code)}
		}
		if !supports(provider.Source, capabilityName) {
			return nil, fmt.Errorf("%w: source %q does not support %s", domain.ErrUnsupportedCapability, code, capabilityName)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func supports(source domain.Source, capabilityName domain.Capability) bool {
	for _, item := range source.Capabilities {
		if item == capabilityName {
			return true
		}
	}
	return false
}

func validateCapabilities(provider Provider) error {
	for _, item := range provider.Source.Capabilities {
		switch item {
		case domain.CapabilityEvents:
			if provider.Events == nil {
				return fmt.Errorf("events capability has no implementation")
			}
		case domain.CapabilityEntityLookup:
			if provider.EntityLookup == nil {
				return fmt.Errorf("entity lookup capability has no implementation")
			}
		case domain.CapabilityArtifactAnalysis:
			if provider.ArtifactAnalyzer == nil {
				return fmt.Errorf("artifact analysis capability has no implementation")
			}
		case domain.CapabilityEndpoints:
			if provider.Endpoints == nil {
				return fmt.Errorf("endpoints capability has no implementation")
			}
		case domain.CapabilityResponseCatalog:
			if provider.ResponseCatalog == nil {
				return fmt.Errorf("response catalog capability has no implementation")
			}
		default:
			return fmt.Errorf("unknown capability %q", item)
		}
	}
	return nil
}
