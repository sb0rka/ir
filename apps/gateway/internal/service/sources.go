package service

import (
	"context"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (service *Service) Supports(sourceCode string, capabilityName domain.Capability) bool {
	provider, ok := service.registry.Provider(sourceCode)
	if !ok {
		return false
	}
	for _, item := range provider.Source.Capabilities {
		if item == capabilityName {
			return true
		}
	}
	return false
}

func (service *Service) ListSources(ctx context.Context, access ProjectAccess, allowedSources []string) []domain.Source {
	allowed := make(map[string]bool, len(allowedSources))
	for _, source := range allowedSources {
		allowed[source] = true
	}
	registered := service.registry.Sources()
	items := make([]domain.Source, 0, len(registered))
	for _, source := range registered {
		if allowed[source.Code] {
			items = append(items, source)
		}
	}

	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	type probeResult struct {
		index  int
		online bool
	}
	results := make(chan probeResult, len(items))
	for index := range items {
		index := index
		go func() {
			provider, ok := service.registry.Provider(items[index].Code)
			if !ok || provider.AccountUserinfo == nil {
				results <- probeResult{index: index, online: ok && provider.Source.Mode == "mock"}
				return
			}
			_, err := service.accountUserinfo(requestCtx, access, provider, false)
			results <- probeResult{index: index, online: err == nil || provider.Source.Mode == "mock"}
		}()
	}
	for range items {
		result := <-results
		if result.online {
			items[result.index].Status = "online"
		} else {
			items[result.index].Status = "offline"
		}
	}
	return items
}
