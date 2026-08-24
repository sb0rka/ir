package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type SearchSessionsRequest struct {
	Sources   []string
	TimeRange domain.TimeRange
	Limit     int
	Cursor    string
}

type SearchSessionsResult struct {
	Sessions     []domain.Session
	NextCursor   string
	SourceStates []domain.SourceState
	SourceErrors []domain.SourceError
}

func (service *Service) SearchSessions(ctx context.Context, access ProjectAccess, request SearchSessionsRequest) (SearchSessionsResult, error) {
	selectedProviders, err := service.registry.Select(request.Sources, domain.CapabilitySessions)
	if err != nil {
		return SearchSessionsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := objectFingerprint(request.Sources, []string{"nad_session"}, request.TimeRange)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchSessionsResult{}, err
	}
	positions := state.Positions
	state.Positions = make(map[string]string)
	providers := pendingProviders(selectedProviders, state.Terminal)
	if len(providers) == 0 {
		return SearchSessionsResult{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is exhausted"}
	}

	type providerResult struct {
		source string
		page   capability.SessionPage
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			var page capability.SessionPage
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				page, innerErr = provider.Sessions.SearchSessions(attemptCtx, providerAccess, capability.SearchSessionsRequest{
					TimeRange: request.TimeRange, Limit: limit, Cursor: positions[provider.Source.Code],
				})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchSessionsResult{}
	for _, provider := range selectedProviders {
		if status := state.Terminal[provider.Source.Code]; status != "" {
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: provider.Source.Code, Status: status})
		}
	}
	pending := providerCodes(providers)
	for len(pending) > 0 {
		select {
		case item := <-results:
			delete(pending, item.source)
			if item.err != nil {
				result.SourceErrors = append(result.SourceErrors, sourceError(item.source, item.err))
				result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: "failed"})
				continue
			}
			status := item.page.Status
			if status == "" {
				status = "complete"
			}
			result.SourceStates = append(result.SourceStates, domain.SourceState{Source: item.source, Status: status})
			result.Sessions = append(result.Sessions, item.page.Sessions...)
			if item.page.NextCursor != "" {
				state.Positions[item.source] = item.page.NextCursor
			} else {
				state.Terminal[item.source] = status
			}
		case <-requestCtx.Done():
			for source := range pending {
				result.SourceStates = append(result.SourceStates, domain.SourceState{Source: source, Status: "failed"})
			}
			appendPendingErrors(&result.SourceErrors, pending, requestCtx.Err())
			pending = nil
		}
	}
	if len(result.SourceErrors) == len(selectedProviders) {
		return SearchSessionsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	result.Sessions = normalizeSessions(result.Sessions)
	if len(result.Sessions) > limit {
		result.Sessions = result.Sessions[:limit]
		markCompleteStatesTruncated(result.SourceStates)
		state.Positions = make(map[string]string)
	}
	if len(state.Positions) > 0 {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchSessionsResult{}, err
		}
	}
	sortSourceStates(result.SourceStates)
	sortSourceErrors(result.SourceErrors)
	return result, nil
}

func (service *Service) GetSession(ctx context.Context, access ProjectAccess, ref domain.SourceObjectRef) (domain.Session, domain.ObjectResolution, error) {
	provider, ok := service.registry.Provider(ref.SourceCode)
	if !ok {
		return domain.Session{}, domain.ObjectResolution{}, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", ref.SourceCode)}
	}
	if provider.Sessions == nil {
		return domain.Session{}, domain.ObjectResolution{}, fmt.Errorf("%w: source %q does not support sessions", domain.ErrUnsupportedCapability, ref.SourceCode)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	var page capability.ContextPage
	err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
		var innerErr error
		page, innerErr = provider.Sessions.ResolveSession(attemptCtx, providerAccess, ref)
		return innerErr
	})
	if err != nil {
		return domain.Session{}, domain.ObjectResolution{}, err
	}
	for _, session := range page.Sessions {
		if sameObjectIdentity(session.Ref, ref) {
			return session, resolutionFor(page.Resolutions, ref), nil
		}
	}
	return domain.Session{}, domain.ObjectResolution{}, fmt.Errorf("%w: session %q", domain.ErrNotFound, ref.ExternalID)
}

func normalizeSessions(items []domain.Session) []domain.Session {
	seen := make(map[string]domain.Session, len(items))
	for _, item := range items {
		seen[objectIdentity(item.Ref)] = item
	}
	result := make([]domain.Session, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].StartedAt.After(result[j].StartedAt)
		}
		return objectIdentity(result[i].Ref) < objectIdentity(result[j].Ref)
	})
	return result
}
