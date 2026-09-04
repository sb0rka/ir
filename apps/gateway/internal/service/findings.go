package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type SearchFindingsRequest struct {
	Sources   []string
	Kinds     []string
	TimeRange domain.TimeRange
	Limit     int
	Cursor    string
}

type SearchFindingsResult struct {
	Findings     []domain.Finding
	NextCursor   string
	Total        *int64
	SourceStates []domain.SourceState
	SourceErrors []domain.SourceError
}

func (service *Service) SearchFindings(ctx context.Context, access ProjectAccess, request SearchFindingsRequest) (SearchFindingsResult, error) {
	selectedProviders, err := service.registry.Select(request.Sources, domain.CapabilityFindings)
	if err != nil {
		return SearchFindingsResult{}, err
	}
	limit := normalizeLimit(request.Limit)
	fingerprint := objectFingerprint(request.Sources, request.Kinds, request.TimeRange)
	state, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return SearchFindingsResult{}, err
	}
	positions := state.Positions
	state.Positions = make(map[string]string)
	providers := pendingProviders(selectedProviders, state.Terminal)
	if len(providers) == 0 {
		return SearchFindingsResult{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is exhausted"}
	}

	type providerResult struct {
		source string
		page   capability.FindingPage
		err    error
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	results := make(chan providerResult, len(providers))
	for _, provider := range providers {
		provider := provider
		go func() {
			var page capability.FindingPage
			callErr := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
				var innerErr error
				page, innerErr = provider.Findings.SearchFindings(attemptCtx, providerAccess, capability.SearchFindingsRequest{
					TimeRange: request.TimeRange, Kinds: request.Kinds, Limit: limit, Cursor: positions[provider.Source.Code],
				})
				return innerErr
			})
			results <- providerResult{source: provider.Source.Code, page: page, err: callErr}
		}()
	}

	result := SearchFindingsResult{}
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
			result.SourceStates = append(result.SourceStates, domain.SourceState{
				Source: item.source, Status: status, Total: item.page.Total,
			})
			result.Findings = append(result.Findings, item.page.Findings...)
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
		return SearchFindingsResult{}, &AllSourcesError{Items: result.SourceErrors}
	}
	result.Findings = normalizeFindings(result.Findings)
	if len(result.Findings) > limit {
		result.Findings = result.Findings[:limit]
		markCompleteStatesTruncated(result.SourceStates)
		// A provider cursor points past its whole vendor page. Once a multi-source
		// merge drops records from that page, advancing it would lose data. Keep
		// the available first page and honestly stop pagination instead.
		state.Positions = make(map[string]string)
	}
	if len(state.Positions) > 0 {
		state.Fingerprint = fingerprint
		result.NextCursor, err = encodeCursor(state)
		if err != nil {
			return SearchFindingsResult{}, err
		}
	}
	sortSourceStates(result.SourceStates)
	sortSourceErrors(result.SourceErrors)
	result.Total = sumSourceTotals(result.SourceStates)
	return result, nil
}

func (service *Service) GetFinding(ctx context.Context, access ProjectAccess, ref domain.SourceObjectRef) (domain.Finding, domain.ObjectResolution, error) {
	provider, ok := service.registry.Provider(ref.SourceCode)
	if !ok {
		return domain.Finding{}, domain.ObjectResolution{}, &domain.RequestError{Code: "source_not_found", Message: fmt.Sprintf("source %q is not registered", ref.SourceCode)}
	}
	if provider.Findings == nil {
		return domain.Finding{}, domain.ObjectResolution{}, fmt.Errorf("%w: source %q does not support findings", domain.ErrUnsupportedCapability, ref.SourceCode)
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	var page capability.ContextPage
	err := service.callProvider(requestCtx, access, provider, func(attemptCtx context.Context, providerAccess capability.Access) error {
		var innerErr error
		page, innerErr = provider.Findings.ResolveFinding(attemptCtx, providerAccess, ref)
		return innerErr
	})
	if err != nil {
		markPagePartial(&page, ref, sourceError(ref.SourceCode, err))
		for _, finding := range page.Findings {
			if sameObjectIdentity(finding.Ref, ref) {
				return finding, resolutionFor(page.Resolutions, ref), nil
			}
		}
		return domain.Finding{}, domain.ObjectResolution{}, err
	}
	for _, finding := range page.Findings {
		if sameObjectIdentity(finding.Ref, ref) {
			resolution := resolutionFor(page.Resolutions, ref)
			return finding, resolution, nil
		}
	}
	return domain.Finding{}, domain.ObjectResolution{}, fmt.Errorf("%w: finding %q", domain.ErrNotFound, ref.ExternalID)
}

func objectFingerprint(sources, kinds []string, value domain.TimeRange) string {
	sources = append([]string(nil), sources...)
	kinds = append([]string(nil), kinds...)
	sort.Strings(sources)
	sort.Strings(kinds)
	raw := strings.Join(sources, ",") + "\x00" + strings.Join(kinds, ",") + "\x00" +
		value.From.UTC().Format(time.RFC3339Nano) + "\x00" + value.To.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeFindings(items []domain.Finding) []domain.Finding {
	seen := make(map[string]domain.Finding, len(items))
	for _, item := range items {
		seen[objectIdentity(item.Ref)] = item
	}
	result := make([]domain.Finding, 0, len(seen))
	for _, item := range seen {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].OccurredAt.Equal(result[j].OccurredAt) {
			return result[i].OccurredAt.After(result[j].OccurredAt)
		}
		return objectIdentity(result[i].Ref) < objectIdentity(result[j].Ref)
	})
	return result
}

func objectIdentity(ref domain.SourceObjectRef) string {
	return ref.SourceCode + "\x00" + ref.SourceInstance + "\x00" + ref.RecordType + "\x00" + ref.ExternalID
}

func sameObjectIdentity(left, right domain.SourceObjectRef) bool {
	return objectIdentity(left) == objectIdentity(right)
}

func resolutionFor(items []domain.ObjectResolution, ref domain.SourceObjectRef) domain.ObjectResolution {
	for _, item := range items {
		if sameObjectIdentity(item.Ref, ref) {
			if item.Status == "" {
				item.Status = "complete"
			}
			return item
		}
	}
	return domain.ObjectResolution{Ref: ref, Status: "complete", Errors: []domain.SourceError{}}
}
