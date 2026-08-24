package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type ResolveContextRequest struct {
	Findings []domain.SourceObjectRef
	Sessions []domain.SourceObjectRef
	Events   []domain.EventSourceRef
	Entities []domain.EntitySourceRef
}

type ResolveContextResult struct {
	Findings     []domain.Finding
	Sessions     []domain.Session
	Events       []domain.Event
	Entities     []domain.Entity
	Relations    []domain.Relation
	Resolutions  []domain.ObjectResolution
	SourceErrors []domain.SourceError
}

type sourceContextRequest struct {
	findings []domain.SourceObjectRef
	sessions []domain.SourceObjectRef
	raw      capability.ResolveContextRequest
}

// ResolveContext keeps every root object returned by a provider even when its
// child context is partial. A missing root contributes only a safe source error
// and is never materialized as an empty canonical object.
func (service *Service) ResolveContext(ctx context.Context, access ProjectAccess, request ResolveContextRequest) (ResolveContextResult, error) {
	bySource := make(map[string]sourceContextRequest)
	for _, ref := range request.Findings {
		item := bySource[ref.SourceCode]
		item.findings = append(item.findings, ref)
		bySource[ref.SourceCode] = item
	}
	for _, ref := range request.Sessions {
		item := bySource[ref.SourceCode]
		item.sessions = append(item.sessions, ref)
		bySource[ref.SourceCode] = item
	}
	for _, ref := range request.Events {
		item := bySource[ref.SourceCode]
		item.raw.EventIDs = append(item.raw.EventIDs, ref.SourceEventID)
		bySource[ref.SourceCode] = item
	}
	for _, ref := range request.Entities {
		item := bySource[ref.SourceCode]
		item.raw.EntityIDs = append(item.raw.EntityIDs, ref.SourceEntityID)
		bySource[ref.SourceCode] = item
	}

	sources := make([]string, 0, len(bySource))
	providers := make(map[string]registry.Provider, len(bySource))
	for source, group := range bySource {
		provider, ok := service.registry.Provider(source)
		if !ok {
			return ResolveContextResult{}, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", source)}
		}
		if len(group.findings) > 0 && provider.Findings == nil {
			return ResolveContextResult{}, fmt.Errorf("%w: source %q does not support findings", domain.ErrUnsupportedCapability, source)
		}
		if len(group.sessions) > 0 && provider.Sessions == nil {
			return ResolveContextResult{}, fmt.Errorf("%w: source %q does not support sessions", domain.ErrUnsupportedCapability, source)
		}
		if (len(group.raw.EventIDs) > 0 || len(group.raw.EntityIDs) > 0) && provider.Events == nil {
			return ResolveContextResult{}, fmt.Errorf("%w: source %q does not support event resolution", domain.ErrUnsupportedCapability, source)
		}
		sources = append(sources, source)
		providers[source] = provider
	}
	sort.Strings(sources)

	type providerResult struct {
		source string
		page   capability.ContextPage
		err    error
		done   bool
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	resultCapacity := len(sources)
	for _, group := range bySource {
		resultCapacity += len(group.findings) + len(group.sessions)
		if len(group.raw.EventIDs) > 0 || len(group.raw.EntityIDs) > 0 {
			resultCapacity++
		}
	}
	results := make(chan providerResult, resultCapacity)
	for _, source := range sources {
		source, provider, group := source, providers[source], bySource[source]
		go func() {
			for _, ref := range group.findings {
				var page capability.ContextPage
				err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
					var innerErr error
					page, innerErr = provider.Findings.ResolveFinding(attemptCtx, providerAccess, ref)
					return innerErr
				})
				if err != nil {
					markPagePartial(&page, ref, sourceError(source, err))
				} else {
					ensurePageResolution(&page, ref)
				}
				results <- providerResult{source: source, page: page, err: err}
			}
			for _, ref := range group.sessions {
				var page capability.ContextPage
				err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
					var innerErr error
					page, innerErr = provider.Sessions.ResolveSession(attemptCtx, providerAccess, ref)
					return innerErr
				})
				if err != nil {
					markPagePartial(&page, ref, sourceError(source, err))
				} else {
					ensurePageResolution(&page, ref)
				}
				results <- providerResult{source: source, page: page, err: err}
			}
			if len(group.raw.EventIDs) > 0 || len(group.raw.EntityIDs) > 0 {
				var page capability.ContextPage
				err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
					var innerErr error
					page, innerErr = provider.Events.ResolveContext(attemptCtx, providerAccess, group.raw)
					return innerErr
				})
				results <- providerResult{source: source, page: page, err: err}
			}
			results <- providerResult{source: source, done: true}
		}()
	}

	result := ResolveContextResult{}
	pending := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		pending[source] = struct{}{}
	}
	for len(pending) > 0 {
		item := <-results
		if item.done {
			delete(pending, item.source)
			continue
		}
		appendContextPage(&result, item.page)
		if item.err != nil {
			result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
		}
	}

	result.Findings = normalizeFindings(result.Findings)
	result.Sessions = normalizeSessions(result.Sessions)
	result.Events = normalization.Events(result.Events)
	result.Entities = normalization.Entities(result.Entities)
	result.Relations = normalization.Relations(result.Relations)
	result.Resolutions = normalizeResolutions(result.Resolutions)
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func appendContextPage(result *ResolveContextResult, page capability.ContextPage) {
	result.Findings = append(result.Findings, page.Findings...)
	result.Sessions = append(result.Sessions, page.Sessions...)
	result.Events = append(result.Events, page.Events...)
	result.Entities = append(result.Entities, page.Entities...)
	result.Relations = append(result.Relations, page.Relations...)
	result.Resolutions = append(result.Resolutions, page.Resolutions...)
}

func markPagePartial(page *capability.ContextPage, ref domain.SourceObjectRef, item domain.SourceError) {
	rootFound := false
	for _, finding := range page.Findings {
		rootFound = rootFound || sameObjectIdentity(finding.Ref, ref)
	}
	for _, session := range page.Sessions {
		rootFound = rootFound || sameObjectIdentity(session.Ref, ref)
	}
	if !rootFound {
		return
	}
	for index := range page.Resolutions {
		if sameObjectIdentity(page.Resolutions[index].Ref, ref) {
			page.Resolutions[index].Status = "partial"
			page.Resolutions[index].Errors = append(page.Resolutions[index].Errors, item)
			return
		}
	}
	page.Resolutions = append(page.Resolutions, domain.ObjectResolution{Ref: ref, Status: "partial", Errors: []domain.SourceError{item}})
}

func ensurePageResolution(page *capability.ContextPage, ref domain.SourceObjectRef) {
	rootFound := false
	for _, finding := range page.Findings {
		rootFound = rootFound || sameObjectIdentity(finding.Ref, ref)
	}
	for _, session := range page.Sessions {
		rootFound = rootFound || sameObjectIdentity(session.Ref, ref)
	}
	if !rootFound {
		return
	}
	for _, resolution := range page.Resolutions {
		if sameObjectIdentity(resolution.Ref, ref) {
			return
		}
	}
	page.Resolutions = append(page.Resolutions, domain.ObjectResolution{Ref: ref, Status: "complete", Errors: []domain.SourceError{}})
}

func normalizeResolutions(items []domain.ObjectResolution) []domain.ObjectResolution {
	seen := make(map[string]domain.ObjectResolution, len(items))
	for _, item := range items {
		key := objectIdentity(item.Ref)
		current, ok := seen[key]
		if !ok || item.Status == "partial" {
			seen[key] = item
			continue
		}
		current.Errors = append(current.Errors, item.Errors...)
		seen[key] = current
	}
	result := make([]domain.ObjectResolution, 0, len(seen))
	for _, item := range seen {
		if item.Status == "" {
			item.Status = "complete"
		}
		if item.Errors == nil {
			item.Errors = []domain.SourceError{}
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return objectIdentity(result[i].Ref) < objectIdentity(result[j].Ref) })
	return result
}
