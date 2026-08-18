package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (service *Service) AnalyzeArtifact(ctx context.Context, source string, request capability.AnalyzeArtifactRequest) (domain.Analysis, error) {
	provider, ok := service.registry.Provider(source)
	if !ok {
		return domain.Analysis{}, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", source)}
	}
	if provider.ArtifactAnalyzer == nil {
		return domain.Analysis{}, fmt.Errorf("%w: source %q does not support artifact analysis", domain.ErrUnsupportedCapability, source)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
	defer cancel()
	return provider.ArtifactAnalyzer.AnalyzeArtifact(requestCtx, request)
}

func (service *Service) GetAnalysis(ctx context.Context, sources []string, analysisID string) (domain.Analysis, error) {
	providers, err := service.registry.Select(sources, domain.CapabilityArtifactAnalysis)
	if err != nil {
		return domain.Analysis{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	for _, provider := range providers {
		sourceCtx, sourceCancel := context.WithTimeout(requestCtx, service.sourceTimeout)
		analysis, callErr := provider.ArtifactAnalyzer.GetAnalysis(sourceCtx, analysisID)
		sourceCancel()
		if callErr == nil {
			return analysis, nil
		}
		if !errors.Is(callErr, domain.ErrNotFound) {
			return domain.Analysis{}, callErr
		}
	}
	return domain.Analysis{}, fmt.Errorf("%w: analysis %q", domain.ErrNotFound, analysisID)
}
