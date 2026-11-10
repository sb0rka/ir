package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

type AgentAccessRepository interface {
	Verify(context.Context, string, string) (agentaccess.Record, error)
	Configuration(context.Context, string) (agentaccess.Configuration, error)
}

type AgentConfigurationCipher interface {
	Open(agentaccess.Configuration) (irconfig.Document, error)
}

type AgentAccess struct {
	Repository AgentAccessRepository
	Cipher     AgentConfigurationCipher
}

type HandlerOption func(*handlerDependencies)

type handlerDependencies struct {
	agentAccess *AgentAccess
}

func WithAgentAccess(repository AgentAccessRepository, cipher AgentConfigurationCipher) HandlerOption {
	return func(dependencies *handlerDependencies) {
		dependencies.agentAccess = &AgentAccess{Repository: repository, Cipher: cipher}
	}
}

type agentAccessContextKey struct{}
type agentRequestAccess struct {
	bearer  string
	project string
}

func (access *AgentAccess) serve(next http.Handler, w http.ResponseWriter, r *http.Request, raw string) {
	project := strings.TrimSpace(r.Header.Get(projectIDHeader))
	if !agentaccess.IsToken(raw) || len(r.Header.Values(projectIDHeader)) != 1 || !validProjectID(project) {
		respondError(w, http.StatusUnauthorized, "unauthorized", "valid agent bearer and project required")
		return
	}
	if access == nil || access.Repository == nil || access.Cipher == nil {
		respondError(w, http.StatusServiceUnavailable, "agent_access_unavailable", "agent authorization unavailable")
		return
	}
	if _, err := access.Repository.Verify(r.Context(), project, raw); err != nil {
		if errors.Is(err, agentaccess.ErrUnauthorized) || errors.Is(err, agentaccess.ErrNotFound) {
			respondError(w, http.StatusUnauthorized, "unauthorized", "invalid agent bearer")
		} else {
			respondError(w, http.StatusServiceUnavailable, "agent_access_unavailable", "agent authorization unavailable")
		}
		return
	}
	stored, err := access.Repository.Configuration(r.Context(), project)
	if err != nil {
		message := "server source configuration unavailable"
		if errors.Is(err, agentaccess.ErrConfigurationMissing) || errors.Is(err, agentaccess.ErrNotFound) {
			message = "server source configuration is not configured"
		}
		respondError(w, http.StatusServiceUnavailable, "configuration_unavailable", message)
		return
	}
	document, err := access.Cipher.Open(stored)
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, "configuration_unavailable", "server source configuration unavailable")
		return
	}
	ctx := common.WithResolvedConfiguration(r.Context(), raw, project, document)
	ctx = context.WithValue(ctx, agentAccessContextKey{}, agentRequestAccess{bearer: raw, project: project})
	next.ServeHTTP(w, r.WithContext(ctx))
}
