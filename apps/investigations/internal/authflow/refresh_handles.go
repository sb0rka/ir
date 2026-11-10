package authflow

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"time"
)

const localRefreshGrace = 30 * time.Second

func (s *Service) RefreshRotating(ctx context.Context, handle string) (model.LoginSession, string, string, error) {
	session, access, err := s.Refresh(ctx, handle)
	if errors.Is(err, ErrUnauthorized) {
		return s.replayLocalRefresh(ctx, handle)
	}
	if err != nil {
		return session, "", "", err
	}
	next, err := randomToken()
	if err != nil {
		return session, "", "", ErrUnavailable
	}
	encrypted, err := seal(s.sessionCipher, "ir:refresh:"+session.ID, next)
	if err != nil {
		return session, "", "", ErrUnavailable
	}
	won, err := s.db.RotateLoginHandle(ctx, session.ID, hash(handle), hash(next), encrypted)
	if err != nil {
		return session, "", "", ErrUnavailable
	}
	if !won {
		return s.replayLocalRefresh(ctx, handle)
	}
	return session, access, next, nil
}

func (s *Service) replayLocalRefresh(ctx context.Context, handle string) (model.LoginSession, string, string, error) {
	if !canonicalRandom(handle) {
		return model.LoginSession{}, "", "", ErrUnauthorized
	}
	previous, err := s.db.PreviousLoginRefresh(ctx, hash(handle))
	if err != nil {
		return model.LoginSession{}, "", "", upstreamError(err)
	}
	if time.Since(previous.RotatedAt) > localRefreshGrace {
		if err := s.db.RevokeLoginSession(ctx, previous.Session.ID); err != nil {
			return previous.Session, "", "", ErrUnavailable
		}
		select {
		case s.revocationWake <- struct{}{}:
		default:
		}
		return previous.Session, "", "", ErrUnauthorized
	}
	var current string
	if open(s.sessionCipher, "ir:refresh:"+previous.Session.ID, previous.EncryptedHandle, &current) != nil || !canonicalRandom(current) || subtle.ConstantTimeCompare(hash(current), previous.Session.RefreshHash) != 1 {
		return previous.Session, "", "", ErrUnauthorized
	}
	session, access, err := s.Refresh(ctx, current)
	return session, access, current, err
}
