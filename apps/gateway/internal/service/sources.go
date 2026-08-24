package service

import (
	"context"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const sourceStatusTTL = 15 * time.Second

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
		status string
	}
	results := make(chan probeResult, len(items))
	for index := range items {
		index := index
		go func() {
			key := credentialKey{projectID: access.ProjectID, sourceCode: items[index].Code}
			if status, ok := service.cachedSourceStatus(key); ok {
				results <- probeResult{index: index, status: status}
				return
			}
			provider, ok := service.registry.Provider(items[index].Code)
			if !ok || provider.Prober == nil {
				service.cacheSourceStatus(key, "offline")
				results <- probeResult{index: index, status: "offline"}
				return
			}
			status := "offline"
			err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				status, innerErr = provider.Prober.Probe(attemptCtx, providerAccess)
				return innerErr
			})
			if err != nil || (status != "online" && status != "degraded") {
				status = "offline"
			}
			service.cacheSourceStatus(key, status)
			results <- probeResult{index: index, status: status}
		}()
	}
	for range items {
		result := <-results
		items[result.index].Status = result.status
	}
	return items
}

func (service *Service) cachedSourceStatus(key credentialKey) (string, bool) {
	service.statusMu.Lock()
	defer service.statusMu.Unlock()
	item, ok := service.statuses[key]
	if !ok || time.Now().After(item.expiresAt) {
		delete(service.statuses, key)
		return "", false
	}
	return item.status, true
}

func (service *Service) cacheSourceStatus(key credentialKey, status string) {
	service.statusMu.Lock()
	defer service.statusMu.Unlock()
	now := time.Now()
	if len(service.statuses) >= credentialCacheLimit {
		for cachedKey, item := range service.statuses {
			if now.After(item.expiresAt) {
				delete(service.statuses, cachedKey)
			}
		}
	}
	if len(service.statuses) >= credentialCacheLimit {
		for cachedKey := range service.statuses {
			delete(service.statuses, cachedKey)
			break
		}
	}
	service.statuses[key] = sourceStatusSnapshot{status: status, expiresAt: now.Add(sourceStatusTTL)}
}
