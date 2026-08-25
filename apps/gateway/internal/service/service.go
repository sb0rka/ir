package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const credentialCacheLimit = 256

type SecretResolver interface {
	Resolve(ctx context.Context, bearer, projectID string, names ...string) (map[string]string, error)
}

type ProjectAccess struct {
	ProjectID string
	Bearer    string
}

type credentialKey struct {
	projectID  string
	sourceCode string
}

type credentialSnapshot struct {
	cookie string
}

type credentialLoad struct {
	mu   sync.Mutex
	refs int
}

type sourceStatusSnapshot struct {
	status    string
	expiresAt time.Time
}

type Service struct {
	registry        *registry.Registry
	secrets         SecretResolver
	requestTimeout  time.Duration
	sourceTimeout   time.Duration
	cacheMu         sync.Mutex
	credentials     map[credentialKey]credentialSnapshot
	credentialKeys  []credentialKey
	credentialLoads map[credentialKey]*credentialLoad
	statusMu        sync.Mutex
	statuses        map[credentialKey]sourceStatusSnapshot
}

func New(registry *registry.Registry, secrets SecretResolver, requestTimeout, sourceTimeout time.Duration) *Service {
	return &Service{
		registry:        registry,
		secrets:         secrets,
		requestTimeout:  requestTimeout,
		sourceTimeout:   sourceTimeout,
		credentials:     make(map[credentialKey]credentialSnapshot),
		credentialLoads: make(map[credentialKey]*credentialLoad),
		statuses:        make(map[credentialKey]sourceStatusSnapshot),
	}
}

type AllSourcesError struct {
	Items []domain.SourceError
}

func (err *AllSourcesError) Error() string { return domain.ErrAllSourcesFailed.Error() }
func (err *AllSourcesError) Unwrap() error { return domain.ErrAllSourcesFailed }

type safeProviderError struct {
	public    error
	retryable bool
}

func (err *safeProviderError) Error() string   { return err.public.Error() }
func (err *safeProviderError) Unwrap() error   { return err.public }
func (err *safeProviderError) Retryable() bool { return err.retryable }

type cursorState struct {
	Fingerprint string            `json:"fingerprint"`
	Positions   map[string]string `json:"positions"`
	Terminal    map[string]string `json:"terminal,omitempty"`
}

func decodeCursor(raw, fingerprint string) (cursorState, error) {
	state := cursorState{Fingerprint: fingerprint, Positions: map[string]string{}, Terminal: map[string]string{}}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(decoded, &state) != nil || state.Positions == nil {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is invalid"}
	}
	if state.Terminal == nil {
		state.Terminal = map[string]string{}
	}
	if state.Fingerprint != fingerprint {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor does not match the request"}
	}
	return state, nil
}

func pendingProviders(providers []registry.Provider, terminal map[string]string) []registry.Provider {
	result := make([]registry.Provider, 0, len(providers))
	for _, provider := range providers {
		if terminal[provider.Source.Code] == "" {
			result = append(result, provider)
		}
	}
	return result
}

func encodeCursor(state cursorState) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func normalizeLimit(value int) int {
	if value < 1 || value > 100 {
		return 50
	}
	return value
}

func sourceError(source string, err error) domain.SourceError {
	item := domain.SourceError{Source: source, Code: "provider_error", Message: "source request failed", Retryable: retryableProviderError(context.Background(), err)}
	var marked interface{ Retryable() bool }
	if errors.As(err, &marked) {
		item.Retryable = marked.Retryable()
	}
	if errors.Is(err, domain.ErrNotFound) {
		item.Code = "source_record_not_found"
		item.Message = "source record was not found"
		item.Retryable = false
		return item
	}
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		item.Code = requestErr.Code
		item.Message = requestErr.Message
		return item
	}
	var upstream *domain.UpstreamError
	if errors.As(err, &upstream) && (upstream.StatusCode == 401 || upstream.StatusCode == 403) {
		item.Code = "source_auth_failed"
		item.Message = "source authentication failed"
		item.Retryable = false
		return item
	}
	if errors.Is(err, context.DeadlineExceeded) {
		item.Code = "source_timeout"
		item.Message = "source request timed out"
	}
	return item
}

func sortSourceErrors(items []domain.SourceError) {
	sort.Slice(items, func(i, j int) bool { return items[i].Source < items[j].Source })
}

func providerCodes(providers []registry.Provider) map[string]struct{} {
	result := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		result[provider.Source.Code] = struct{}{}
	}
	return result
}

func appendPendingErrors(items *[]domain.SourceError, pending map[string]struct{}, err error) {
	for source := range pending {
		*items = append(*items, sourceError(source, err))
	}
}

// loadCredentials serializes Secret resolution per project/source key. Different
// providers resolve concurrently so one slow Secret cannot consume another
// provider's request budget.
func (service *Service) loadCredentials(ctx context.Context, access ProjectAccess, provider registry.Provider, force bool) (credentialSnapshot, error) {
	key := credentialKey{projectID: access.ProjectID, sourceCode: provider.Source.Code}
	if force {
		service.invalidateSourceStatus(key)
	}
	release := service.acquireCredentialLoad(key)
	defer release()

	service.cacheMu.Lock()
	if force {
		delete(service.credentials, key)
		for index, cachedKey := range service.credentialKeys {
			if cachedKey == key {
				service.credentialKeys = append(service.credentialKeys[:index], service.credentialKeys[index+1:]...)
				break
			}
		}
	}
	if snapshot, ok := service.credentials[key]; ok {
		service.cacheMu.Unlock()
		return snapshot, nil
	}
	service.cacheMu.Unlock()

	if service.secrets == nil || strings.TrimSpace(access.Bearer) == "" || strings.TrimSpace(provider.CredentialSecret) == "" {
		return credentialSnapshot{}, sourceUnavailable()
	}
	values, err := service.secrets.Resolve(ctx, access.Bearer, access.ProjectID, provider.CredentialSecret)
	if err != nil {
		return credentialSnapshot{}, sourceUnavailable()
	}
	cookie := strings.TrimSpace(values[provider.CredentialSecret])
	if cookie == "" || strings.ContainsAny(cookie, "\r\n") {
		return credentialSnapshot{}, sourceUnavailable()
	}
	snapshot := credentialSnapshot{cookie: cookie}
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()
	if len(service.credentials) >= credentialCacheLimit {
		oldest := service.credentialKeys[0]
		service.credentialKeys = service.credentialKeys[1:]
		delete(service.credentials, oldest)
	}
	service.credentials[key] = snapshot
	service.credentialKeys = append(service.credentialKeys, key)
	return snapshot, nil
}

func (service *Service) acquireCredentialLoad(key credentialKey) func() {
	service.cacheMu.Lock()
	load := service.credentialLoads[key]
	if load == nil {
		load = &credentialLoad{}
		service.credentialLoads[key] = load
	}
	load.refs++
	service.cacheMu.Unlock()

	load.mu.Lock()
	return func() {
		load.mu.Unlock()
		service.cacheMu.Lock()
		defer service.cacheMu.Unlock()
		load.refs--
		if load.refs == 0 {
			delete(service.credentialLoads, key)
		}
	}
}

func sourceUnavailable() error {
	return &domain.RequestError{Code: "source_unavailable", Message: "source credentials are not configured for this project"}
}

func retryableProviderError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var upstream *domain.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.StatusCode == 401 || upstream.StatusCode == 403 ||
			(upstream.StatusCode >= 300 && upstream.StatusCode < 400) || upstream.StatusCode >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func providerError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return err
	}
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		return requestErr
	}
	var upstream *domain.UpstreamError
	if errors.As(err, &upstream) {
		if upstream.StatusCode == 401 || upstream.StatusCode == 403 {
			return &domain.RequestError{Code: "source_auth_failed", Message: "source authentication failed"}
		}
		return &safeProviderError{
			public:    &domain.RequestError{Code: "provider_error", Message: "source request failed"},
			retryable: upstream.StatusCode >= 500,
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return &safeProviderError{
		public:    &domain.RequestError{Code: "provider_error", Message: "source request failed"},
		retryable: retryableProviderError(context.Background(), err),
	}
}

func (service *Service) callProvider(ctx context.Context, access ProjectAccess, provider registry.Provider, call func(context.Context, capability.Access) error) error {
	return service.callProviderWithCredentialReload(ctx, access, provider, false, call)
}

func (service *Service) callProviderWithCredentialReload(ctx context.Context, access ProjectAccess, provider registry.Provider, force bool, call func(context.Context, capability.Access) error) error {
	credentials, err := service.loadCredentials(ctx, access, provider, force)
	if err != nil {
		return err
	}
	attemptCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
	err = call(attemptCtx, capability.Access{Cookie: credentials.cookie})
	cancel()
	if err == nil {
		return nil
	}
	if !retryableProviderError(ctx, err) {
		return providerError(err)
	}
	credentials, reloadErr := service.loadCredentials(ctx, access, provider, true)
	if reloadErr != nil {
		return reloadErr
	}
	attemptCtx, cancel = context.WithTimeout(ctx, service.sourceTimeout)
	err = call(attemptCtx, capability.Access{Cookie: credentials.cookie})
	cancel()
	if err != nil {
		return providerError(err)
	}
	return nil
}
