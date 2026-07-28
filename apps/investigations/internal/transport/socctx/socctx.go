// Package socctx переносит через context то, чего нет в общей личности
// платформы: тенант и роли SOC.
//
// Личность из токена — sub, sid, kind — живёт в coreauthctx и одинакова во
// всех сервисах. Роли l1/l2/lead/admin пока есть только здесь, поэтому лежат
// отдельным значением, а не расширением общего типа. Понадобятся второму
// сервису — переедут в core вместе с резолвером.
package socctx

import "context"

// Scope — то, в каком тенанте действует субъект и что ему там можно.
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

// ScopeFromContext отдаёт тенант и роли. Пустой список ролей до контекста не
// доходит — deny-by-default в middleware отсекает такой запрос раньше.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	if !ok || scope.ProjectID == "" {
		return Scope{}, false
	}
	return scope, true
}
