// Package socctx carries the selected project and the verified request bearer.
package socctx

import (
	"context"
	"strings"
)

type Scope struct {
	ProjectID string
}

type scopeKey struct{}

type AgentAuthorization struct {
	InvestigationID string
	Scope           string
}

type agentAuthorizationKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// Сырой bearer-токен запроса используется только для project-scoped вызовов
// Sb0rka API. Токены внешних инструментов берутся из Secrets и живут в кэше.
type bearerKey struct{}

func WithBearer(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerKey{}, token)
}

func BearerFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(bearerKey{}).(string)
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || scope.ProjectID == "" {
		return Scope{}, false
	}
	return scope, true
}

func WithAgentAuthorization(ctx context.Context, authorization AgentAuthorization) context.Context {
	return context.WithValue(ctx, agentAuthorizationKey{}, authorization)
}

func AgentAuthorizationFromContext(ctx context.Context) (AgentAuthorization, bool) {
	authorization, ok := ctx.Value(agentAuthorizationKey{}).(AgentAuthorization)
	if !ok || authorization.InvestigationID == "" || authorization.Scope == "" {
		return AgentAuthorization{}, false
	}
	return authorization, true
}

func (a AgentAuthorization) HasScope(required string) bool {
	for _, scope := range strings.Fields(a.Scope) {
		if scope == required {
			return true
		}
	}
	return false
}
