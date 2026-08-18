package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Service struct {
	registry       *registry.Registry
	requestTimeout time.Duration
	sourceTimeout  time.Duration
}

func New(registry *registry.Registry, requestTimeout, sourceTimeout time.Duration) *Service {
	return &Service{registry: registry, requestTimeout: requestTimeout, sourceTimeout: sourceTimeout}
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
