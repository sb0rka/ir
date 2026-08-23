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

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type SecretResolver interface {
	Resolve(ctx context.Context, bearer, projectID string, names ...string) (map[string]string, error)
}

type ProjectAccess struct {
	ProjectID string
	Bearer    string
}

type credentialSnapshot struct {
	projectID  string
	sourceCode string
	baseURL    string
	credential string
}

type Service struct {
	registry       *registry.Registry
	secrets        SecretResolver
	requestTimeout time.Duration
	sourceTimeout  time.Duration
	skipTLSVerify    bool
	cacheMu        sync.Mutex
	credentials    credentialSnapshot
}

func New(registry *registry.Registry, secrets SecretResolver, requestTimeout, sourceTimeout time.Duration, skipTLSVerify bool) *Service {
	return &Service{
		registry:       registry,
		secrets:        secrets,
		requestTimeout: requestTimeout,
		sourceTimeout:  sourceTimeout,
		skipTLSVerify:    skipTLSVerify,
	}
}

type AllSourcesError struct {
	Items []domain.SourceError
}

func (err *AllSourcesError) Error() string { return domain.ErrAllSourcesFailed.Error() }
func (err *AllSourcesError) Unwrap() error { return domain.ErrAllSourcesFailed }

type cursorState struct {
	Fingerprint string            `json:"fingerprint"`
	Positions   map[string]string `json:"positions"`
}

func decodeCursor(raw, fingerprint string) (cursorState, error) {
	state := cursorState{Fingerprint: fingerprint, Positions: map[string]string{}}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || json.Unmarshal(decoded, &state) != nil || state.Positions == nil {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor is invalid"}
	}
	if state.Fingerprint != fingerprint {
		return cursorState{}, &domain.RequestError{Code: "invalid_cursor", Message: "cursor does not match the request"}
	}
	return state, nil
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
	item := domain.SourceError{Source: source, Code: "provider_error", Message: "source request failed", Retryable: true}
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		item.Code = requestErr.Code
		item.Message = requestErr.Message
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

func (service *Service) loadCredentials(ctx context.Context, access ProjectAccess, provider registry.Provider, force bool) (credentialSnapshot, error) {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if !force && service.credentials.projectID == access.ProjectID && service.credentials.sourceCode == provider.Source.Code &&
		service.credentials.baseURL != "" && service.credentials.credential != "" {
		return service.credentials, nil
	}
	if service.secrets == nil || strings.TrimSpace(access.Bearer) == "" {
		return credentialSnapshot{}, sourceUnavailable()
	}
	secretNames := provider.AccountUserinfo.SecretNames()
	if strings.TrimSpace(secretNames.BaseURL) == "" || strings.TrimSpace(secretNames.Credential) == "" {
		return credentialSnapshot{}, sourceUnavailable()
	}
	values, err := service.secrets.Resolve(ctx, access.Bearer, access.ProjectID, secretNames.BaseURL, secretNames.Credential)
	if err != nil {
		return credentialSnapshot{}, sourceUnavailable()
	}
	snapshot := credentialSnapshot{
		projectID:  access.ProjectID,
		sourceCode: provider.Source.Code,
		baseURL:    strings.TrimSpace(values[secretNames.BaseURL]),
		credential: strings.TrimSpace(values[secretNames.Credential]),
	}
	if snapshot.projectID == "" || snapshot.baseURL == "" || snapshot.credential == "" {
		return credentialSnapshot{}, sourceUnavailable()
	}
	service.credentials = snapshot
	return snapshot, nil
}

func sourceUnavailable() error {
	return &domain.RequestError{
		Code:    "source_unavailable",
		Message: "source credentials are not configured for this project",
	}
}

func retryableAccountError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var upstream *domain.UpstreamError
	if errors.As(err, &upstream) {
		return upstream.StatusCode == 401 || upstream.StatusCode == 403 || upstream.StatusCode >= 500
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func accountError(err error) error {
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		return requestErr
	}
	var upstream *domain.UpstreamError
	if errors.As(err, &upstream) {
		if upstream.StatusCode == 401 || upstream.StatusCode == 403 {
			return &domain.RequestError{Code: "source_auth_failed", Message: "source authentication failed"}
		}
		return &domain.RequestError{Code: "provider_error", Message: "source account request failed"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return &domain.RequestError{Code: "provider_error", Message: "source account request failed"}
}
