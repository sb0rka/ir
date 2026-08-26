// Package server implements the generated API contracts.
package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/common"
)

type Server struct {
	db  store.Database
	log *slog.Logger

	som     *somclient.Client
	secrets *common.SecretsClient
	somAuth somTokenCache
	gateway *gatewayclient.Client
	prompt  config.PromptConfig

	mcpTokensMu sync.Mutex
	mcpTokens   map[string]mcpCapability
}

type mcpCapability struct {
	ProjectID       string
	InvestigationID string
	ExpiresAt       time.Time
}

type cursorPayload struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

func decodeCursor(value *string) (*model.PageCursor, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*value)
	if err != nil {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "invalid cursor")
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Time.IsZero() {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "invalid cursor")
	}
	if _, err := uuid.Parse(payload.ID); err != nil {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "invalid cursor")
	}
	return &model.PageCursor{Time: payload.Time, ID: payload.ID}, nil
}

func encodeCursor(at time.Time, id string) string {
	raw, _ := json.Marshal(cursorPayload{Time: at, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

var _ transport.API = (*Server)(nil)

func New(
	db store.Database,
	log *slog.Logger,
	som *somclient.Client,
	secrets *common.SecretsClient,
	gateway *gatewayclient.Client,
	prompt config.PromptConfig,
) *Server {
	return &Server{
		db: db, log: log, som: som, secrets: secrets, gateway: gateway, prompt: prompt,
		mcpTokens: make(map[string]mcpCapability),
	}
}

// issueMCPToken creates a short-lived capability for exactly one project and
// investigation. It is not a REST credential and carries no human OAuth token.
func (s *Server) issueMCPToken(projectID, investigationID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate MCP capability: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	s.mcpTokensMu.Lock()
	defer s.mcpTokensMu.Unlock()
	for value, capability := range s.mcpTokens {
		if !capability.ExpiresAt.After(now) {
			delete(s.mcpTokens, value)
		}
	}
	s.mcpTokens[token] = mcpCapability{
		ProjectID: projectID, InvestigationID: investigationID, ExpiresAt: now.Add(4 * time.Hour),
	}
	return token, nil
}

func (s *Server) consumeMCPToken(token string) (mcpCapability, bool) {
	s.mcpTokensMu.Lock()
	defer s.mcpTokensMu.Unlock()
	capability, ok := s.mcpTokens[token]
	if !ok || !capability.ExpiresAt.After(time.Now()) {
		delete(s.mcpTokens, token)
		return mcpCapability{}, false
	}
	return capability, true
}

// Хелперы живут здесь, а не в отдельном файле: gen-prune удаляет из пакета
// любой файл, не названный по домену из api/investigations/paths.

// scope — project scope запроса. Доменные ручки без него не работают:
// project_id участвует в каждом запросе к базе.
func (s *Server) scope(ctx context.Context) (socctx.Scope, error) {
	scope, ok := socctx.ScopeFromContext(ctx)
	if !ok {
		return socctx.Scope{}, httperr.BadRequest("X-Project-ID header is required")
	}
	return scope, nil
}

// storeError переводит сентинелы стора в HTTP-коды контракта.
func storeError(err error) error {
	switch {
	case errors.Is(err, store.ErrInvestigationNotFound), errors.Is(err, store.ErrParentNotFound),
		errors.Is(err, store.ErrRecordNotFound):
		return httperr.ErrNotFound
	case errors.Is(err, store.ErrTargetNotAttached), errors.Is(err, store.ErrUnknownReference):
		return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"a reference does not belong to this project, investigation, or batch")
	case errors.Is(err, store.ErrConflict):
		return httperr.New(http.StatusConflict, httperr.CodeConflict,
			"the record is used by confirmed graph evidence")
	case errors.Is(err, store.ErrInvalidValue):
		return httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"value violates a domain constraint")
	}
	return err
}

// dbUUID парсит id, пришедший из базы. Ошибка здесь — повреждённые данные,
// а не вина клиента.
func dbUUID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, httperr.New(http.StatusInternalServerError, httperr.CodeInternal,
			"storage returned a malformed identifier")
	}
	return parsed, nil
}
