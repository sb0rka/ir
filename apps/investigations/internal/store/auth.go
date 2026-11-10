package store

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

// AuthDatabase keeps network and cryptographic operations outside SQL transactions.
type AuthDatabase interface {
	CreateLoginSession(context.Context, model.LoginSession) error
	LoginSessionByRefresh(context.Context, []byte) (model.LoginSession, error)
	PreviousLoginRefresh(context.Context, []byte) (model.PreviousRefresh, error)
	RotateLoginHandle(context.Context, string, []byte, []byte, []byte) (bool, error)
	LoginSessionByIdentity(context.Context, string, string) (model.LoginSession, error)
	LoginSessionByID(context.Context, string) (model.LoginSession, error)
	ReserveLoginRefresh(context.Context, string, int64, string) (bool, error)
	CompleteLoginRefresh(context.Context, string, int64, string, []byte) (bool, error)
	ReleaseLoginRefresh(context.Context, string, string) error
	RevokeLoginSession(context.Context, string) error
	CreateLoginRequest(context.Context, model.LoginRequest) error
	ConsumeLoginCallback(context.Context, []byte) (model.LoginRequest, error)
	FinishLoginCallback(context.Context, string, []byte, []byte) error
	RedeemLoginHandoff(context.Context, []byte) (model.LoginRequest, error)
	QueueAuthRevocation(context.Context, model.AuthRevocation) error
	ExpireAuthRevocations(context.Context) ([]string, error)
	ClaimAuthRevocations(context.Context, int) ([]model.AuthRevocation, error)
	CompleteAuthRevocation(context.Context, string) error
}
