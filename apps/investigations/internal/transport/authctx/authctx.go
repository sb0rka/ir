// Package authctx переносит личность вызывающего через context.
package authctx

import "context"

type ctxKey int

const identityKey ctxKey = iota

// Identity — то, что сервис знает о вызывающем: субъект из токена платформы
// и роли SOC из role_bindings. Тенант (ProjectID) резолвится по субъекту,
// из тела запроса он никогда не берётся.
type Identity struct {
	SubjectID   string
	SubjectKind string
	SessionID   string
	ProjectID   string
	Roles       []string
}

func (i Identity) HasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func With(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

func From(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}
