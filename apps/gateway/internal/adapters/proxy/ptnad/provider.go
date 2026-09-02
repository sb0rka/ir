package ptnad

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const CredentialSecretName = "DEMO_PT_NAD_COOKIE"

type Provider struct {
	client   *Client
	storeIDs []int64
	stores   map[int64]struct{}
}

var (
	_ capability.FindingSource = (*Provider)(nil)
	_ capability.SessionSource = (*Provider)(nil)
	_ capability.EventSource   = (*Provider)(nil)
	_ capability.EntityLookup  = (*Provider)(nil)
	_ capability.SourceProber  = (*Provider)(nil)
)

func NewProvider(client *Client, storeIDs []int64) (*Provider, error) {
	if client == nil {
		return nil, fmt.Errorf("PT NAD client is required")
	}
	stores := make(map[int64]struct{}, len(storeIDs))
	for _, storeID := range storeIDs {
		if storeID <= 0 {
			return nil, fmt.Errorf("PT NAD store IDs must be positive")
		}
		stores[storeID] = struct{}{}
	}
	if len(stores) == 0 {
		return nil, fmt.Errorf("at least one PT NAD store ID is required")
	}
	normalized := make([]int64, 0, len(stores))
	for storeID := range stores {
		normalized = append(normalized, storeID)
	}
	sort.Slice(normalized, func(left, right int) bool { return normalized[left] < normalized[right] })
	return &Provider{client: client, storeIDs: normalized, stores: stores}, nil
}

// RegistryProvider is the sole composition-root representation of this
// adapter. Vendor URLs and store IDs remain captured in Provider/Client and are
// never accepted through a capability request.
func (provider *Provider) RegistryProvider() registry.Provider {
	return registry.Provider{
		Source: domain.Source{
			Code: SourceCode, Name: "PT NAD", Kind: "ndr", Mode: "proxy", Status: "offline",
			Capabilities: []domain.Capability{
				domain.CapabilityFindings,
				domain.CapabilitySessions,
				domain.CapabilityEvents,
				domain.CapabilityEntityLookup,
			},
		},
		CredentialSecret: CredentialSecretName,
		Findings:         provider,
		Sessions:         provider,
		Events:           provider,
		EntityLookup:     provider,
		Prober:           provider,
	}
}

func (provider *Provider) SearchFindings(ctx context.Context, access capability.Access, request capability.SearchFindingsRequest) (capability.FindingPage, error) {
	if strings.TrimSpace(request.Cursor) != "" {
		return capability.FindingPage{}, invalidRequest("PT NAD does not expose a confirmed finding cursor")
	}
	if err := validateDomainTimeRange(request.TimeRange); err != nil {
		return capability.FindingPage{}, err
	}
	if !wantedKind(request.Kinds, AttackRecordType) {
		return capability.FindingPage{Findings: []domain.Finding{}, Status: "complete"}, nil
	}
	limit, err := validateCapabilityLimit(request.Limit)
	if err != nil {
		return capability.FindingPage{}, err
	}
	results, partial, err := fanOutStores(ctx, provider, func(storeID int64) (AttackSearchResult, error) {
		return provider.client.SearchAttacks(ctx, SearchRequest{
			StoreID: storeID, From: request.TimeRange.From, To: request.TimeRange.To, Limit: limit,
		}, Access{Cookie: access.Cookie})
	})
	if err != nil {
		return capability.FindingPage{}, canonicalProviderError(err)
	}
	page := capability.FindingPage{Status: "complete"}
	seen := make(map[string]struct{})
	for _, result := range results {
		partial = partial || result.Truncated
		for _, attack := range result.Attacks {
			finding := canonicalFinding(attack)
			key := objectIdentity(finding.Ref)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			page.Findings = append(page.Findings, finding)
		}
	}
	sort.Slice(page.Findings, func(left, right int) bool {
		if !page.Findings[left].OccurredAt.Equal(page.Findings[right].OccurredAt) {
			return page.Findings[left].OccurredAt.After(page.Findings[right].OccurredAt)
		}
		return objectIdentity(page.Findings[left].Ref) < objectIdentity(page.Findings[right].Ref)
	})
	if len(page.Findings) > limit {
		page.Findings = page.Findings[:limit]
		partial = true
	}
	if partial {
		page.Status = "truncated"
	}
	return page, nil
}

func (provider *Provider) ResolveFinding(ctx context.Context, access capability.Access, ref domain.SourceObjectRef) (capability.ContextPage, error) {
	storeID, timeRange, err := provider.validateObjectRef(ref, AttackRecordType)
	if err != nil {
		return capability.ContextPage{}, err
	}
	attack, err := provider.client.GetAttack(ctx, AttackRef{
		StoreID: storeID, ExternalID: ref.ExternalID, TimeRange: timeRange,
	}, Access{Cookie: access.Cookie})
	if err != nil {
		return capability.ContextPage{}, canonicalProviderError(err)
	}
	root := canonicalFinding(attack)
	page := capability.ContextPage{
		Findings:    []domain.Finding{root},
		Resolutions: []domain.ObjectResolution{{Ref: root.Ref, Status: "complete", Errors: []domain.SourceError{}}},
	}
	if attack.ParentSession == nil {
		page.Resolutions[0].Status = "partial"
		page.Resolutions[0].Errors = []domain.SourceError{{
			Source: SourceCode, Code: "missing_parent_session", Message: "finding session context is unavailable",
		}}
		appendAttackContext(&page, attack)
		return normalizeContextPage(page), nil
	}
	session, err := provider.client.GetSession(ctx, SessionRef{
		StoreID: storeID, ExternalID: attack.ParentSession.ExternalID, TimeRange: timeRange,
	}, Access{Cookie: access.Cookie})
	if err != nil {
		page.Resolutions[0].Status = "partial"
		warning := contextWarning("finding.session", err)
		page.Resolutions[0].Errors = []domain.SourceError{warning}
		appendAttackContext(&page, attack)
		page = normalizeContextPage(page)
		if warning.Retryable {
			return page, canonicalProviderError(err)
		}
		return page, nil
	}
	appendSessionContext(&page, session)
	for _, contextErr := range session.ContextErrors {
		page.Resolutions[0].Status = "partial"
		page.Resolutions[0].Errors = append(page.Resolutions[0].Errors, contextWarning("finding session HTTP transaction pagination", contextErr))
	}
	// Flow detail carries the reviewed rule metadata. Replace only the matching
	// root snapshot while retaining the exact BQL root when the child omits it.
	rootEnriched := false
	for _, related := range session.RelatedAttacks {
		if related.SourceRef.Identity() == attack.SourceRef.Identity() {
			page.Findings[0] = canonicalFinding(related)
			rootEnriched = true
			break
		}
	}
	if !rootEnriched {
		page.Resolutions[0].Status = "partial"
		page.Resolutions[0].Errors = append(page.Resolutions[0].Errors, domain.SourceError{
			Source: SourceCode, Code: "missing_flow_alert", Message: "finding flow detail is incomplete",
		})
	}
	return normalizeContextPage(page), nil
}

func (provider *Provider) SearchSessions(ctx context.Context, access capability.Access, request capability.SearchSessionsRequest) (capability.SessionPage, error) {
	if strings.TrimSpace(request.Cursor) != "" {
		return capability.SessionPage{}, invalidRequest("PT NAD does not expose a confirmed session cursor")
	}
	if err := validateDomainTimeRange(request.TimeRange); err != nil {
		return capability.SessionPage{}, err
	}
	limit, err := validateCapabilityLimit(request.Limit)
	if err != nil {
		return capability.SessionPage{}, err
	}
	results, partial, err := fanOutStores(ctx, provider, func(storeID int64) (SessionSearchResult, error) {
		return provider.client.SearchSessions(ctx, SearchRequest{
			StoreID: storeID, From: request.TimeRange.From, To: request.TimeRange.To, Limit: limit,
		}, Access{Cookie: access.Cookie})
	})
	if err != nil {
		return capability.SessionPage{}, canonicalProviderError(err)
	}
	page := capability.SessionPage{Status: "complete"}
	seen := make(map[string]struct{})
	for _, result := range results {
		partial = partial || result.Truncated
		for _, session := range result.Sessions {
			item := canonicalSession(session)
			key := objectIdentity(item.Ref)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			page.Sessions = append(page.Sessions, item)
		}
	}
	sort.Slice(page.Sessions, func(left, right int) bool {
		if !page.Sessions[left].StartedAt.Equal(page.Sessions[right].StartedAt) {
			return page.Sessions[left].StartedAt.After(page.Sessions[right].StartedAt)
		}
		return objectIdentity(page.Sessions[left].Ref) < objectIdentity(page.Sessions[right].Ref)
	})
	if len(page.Sessions) > limit {
		page.Sessions = page.Sessions[:limit]
		partial = true
	}
	if partial {
		page.Status = "truncated"
	}
	return page, nil
}

func (provider *Provider) ResolveSession(ctx context.Context, access capability.Access, ref domain.SourceObjectRef) (capability.ContextPage, error) {
	storeID, timeRange, err := provider.validateObjectRef(ref, SessionRecordType)
	if err != nil {
		return capability.ContextPage{}, err
	}
	session, err := provider.client.GetSession(ctx, SessionRef{
		StoreID: storeID, ExternalID: ref.ExternalID, TimeRange: timeRange,
	}, Access{Cookie: access.Cookie})
	if err != nil {
		return capability.ContextPage{}, canonicalProviderError(err)
	}
	page := capability.ContextPage{}
	appendSessionContext(&page, session)
	return normalizeContextPage(page), nil
}

func (provider *Provider) Probe(ctx context.Context, access capability.Access) (string, error) {
	_, partial, err := fanOutStores(ctx, provider, func(storeID int64) (Store, error) {
		return provider.client.GetStore(ctx, storeID, Access{Cookie: access.Cookie})
	})
	if err != nil {
		return "offline", canonicalProviderError(err)
	}
	if partial {
		return "degraded", nil
	}
	return "online", nil
}

func (provider *Provider) validateObjectRef(ref domain.SourceObjectRef, recordType string) (int64, TimeRange, error) {
	if ref.SourceCode != SourceCode || ref.RecordType != recordType {
		return 0, TimeRange{}, invalidRequest("PT NAD object reference has the wrong source or record type")
	}
	storeID, err := strconv.ParseInt(ref.SourceInstance, 10, 64)
	if err != nil || storeID <= 0 || strconv.FormatInt(storeID, 10) != ref.SourceInstance {
		return 0, TimeRange{}, invalidRequest("PT NAD source_instance is invalid")
	}
	if _, configured := provider.stores[storeID]; !configured {
		return 0, TimeRange{}, invalidRequest("PT NAD source_instance is not configured")
	}
	if err := validateExternalID(ref.ExternalID); err != nil {
		return 0, TimeRange{}, invalidRequest("PT NAD external_id is invalid")
	}
	if err := validateDomainTimeRange(ref.TimeRange); err != nil {
		return 0, TimeRange{}, err
	}
	return storeID, TimeRange{From: ref.TimeRange.From.UTC(), To: ref.TimeRange.To.UTC()}, nil
}

type indexedStoreResult[T any] struct {
	index int
	value T
	err   error
}

func fanOutStores[T any](ctx context.Context, provider *Provider, call func(int64) (T, error)) ([]T, bool, error) {
	results := make(chan indexedStoreResult[T], len(provider.storeIDs))
	for index, storeID := range provider.storeIDs {
		index, storeID := index, storeID
		go func() {
			value, err := call(storeID)
			results <- indexedStoreResult[T]{index: index, value: value, err: err}
		}()
	}
	ordered := make([]indexedStoreResult[T], len(provider.storeIDs))
	for range provider.storeIDs {
		select {
		case result := <-results:
			ordered[result.index] = result
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	values := make([]T, 0, len(ordered))
	partial := false
	var firstError error
	for _, result := range ordered {
		if result.err != nil {
			partial = true
			if firstError == nil {
				firstError = result.err
			}
			continue
		}
		values = append(values, result.value)
	}
	if len(values) == 0 {
		return nil, false, firstError
	}
	return values, partial, nil
}

func canonicalProviderError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: PT NAD record", domain.ErrNotFound)
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		return &domain.UpstreamError{
			StatusCode: responseError.StatusCode,
			Message:    fmt.Sprintf("PT NAD upstream returned HTTP %d", responseError.StatusCode),
		}
	}
	return err
}

func contextWarning(component string, err error) domain.SourceError {
	value := domain.SourceError{
		Source: SourceCode, Code: "upstream_failed", Message: component + " context is unavailable",
	}
	var responseError *ResponseError
	if errors.As(err, &responseError) {
		value.Retryable = responseError.StatusCode == 401 || responseError.StatusCode == 403 || responseError.StatusCode >= 500
		if responseError.StatusCode == httpStatusNotFound {
			value.Code = "not_found"
		}
		return value
	}
	var transportError *TransportError
	if errors.As(err, &transportError) {
		value.Code = "transport_failed"
		value.Retryable = true
		return value
	}
	if errors.Is(err, ErrNotFound) {
		value.Code = "not_found"
	}
	return value
}

const httpStatusNotFound = 404

func validateDomainTimeRange(value domain.TimeRange) error {
	if value.From.IsZero() || value.To.IsZero() || !value.From.Before(value.To) {
		return invalidRequest("PT NAD time range must satisfy from < to")
	}
	return nil
}

func validateCapabilityLimit(limit int) (int, error) {
	if limit < 1 || limit > MaxLimit {
		return 0, invalidRequest(fmt.Sprintf("PT NAD limit must be between 1 and %d", MaxLimit))
	}
	return limit, nil
}

func wantedKind(kinds []string, wanted string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, kind := range kinds {
		if kind == wanted {
			return true
		}
	}
	return false
}

func invalidRequest(message string) error {
	return &domain.RequestError{Code: "invalid_request", Message: message}
}
