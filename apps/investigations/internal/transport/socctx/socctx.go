// Package socctx carries the selected project and the verified request bearer.
package socctx

import "context"

type Scope struct {
	ProjectID string
}

type scopeKey struct{}

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
