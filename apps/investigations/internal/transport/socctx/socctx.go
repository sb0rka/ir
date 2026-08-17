// Package socctx несёт project scope и роли SOC — того, чего нет в coreauthctx.
//
// Отдельным значением, а не расширением общего типа: роли l1/l2/lead/admin
// пока есть только здесь. Понадобятся второму сервису — переедут в core.
package socctx

import "context"

type Scope struct {
	ProjectID string
	Roles     []string
}

func (s Scope) HasRole(role string) bool {
	for _, r := range s.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type scopeKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// Сырой bearer-токен запроса. Нужен som-домену: IR не выпускает собственных
// токенов для SOM, а пробрасывает токен вызывающего как есть (pass-through).
// Кладётся до всех проверок, чтобы работал и при AUTH_DISABLED.
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

// Пустой список ролей сюда не доходит: deny-by-default отсекает раньше.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || scope.ProjectID == "" {
		return Scope{}, false
	}
	return scope, true
}
