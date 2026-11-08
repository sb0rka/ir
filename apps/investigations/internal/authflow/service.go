// Package authflow owns IR login sessions and client-bound delegations. It never
// grants integration access as a side effect of login.
package authflow

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("integration consent required")
	ErrUnavailable  = errors.New("authentication service unavailable")
	ErrNotFound     = errors.New("project not found")
	ErrInvalid      = errors.New("invalid authentication request")
)

const tokenTypeURN = "urn:ietf:params:oauth:token-type:access_token"
const exchangeGrant = "urn:ietf:params:oauth:grant-type:token-exchange"
const delegationGrant = "urn:sb0rka:params:oauth:grant-type:delegation"
const authRevocationBatch = 50

type Service struct {
	db             store.AuthDatabase
	cfg            config.Config
	http           *http.Client
	verifier       *oidc.IDTokenVerifier
	oauth          oauth2.Config
	sessionCipher  cipher.AEAD
	contextCipher  cipher.AEAD
	cacheMu        sync.Mutex
	tokens         map[string]cachedToken
	revocationWake chan struct{}
	agentDB        store.AgentAccessDatabase
	agentCipher    *agentaccess.Cipher
}

type cachedToken struct {
	Raw     string
	Expires time.Time
}

type tokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
}

type pendingContext struct {
	Verifier     string `json:"verifier"`
	Nonce        string `json:"nonce"`
	AppChallenge string `json:"app_challenge"`
}

type handoffResult struct {
	SessionID     string `json:"session_id"`
	RefreshHandle string `json:"refresh_handle"`
}

type Grant struct {
	Connected bool     `json:"connected"`
	ID        string   `json:"delegation_id,omitempty"`
	Version   int64    `json:"version,omitempty"`
	Audience  string   `json:"audience,omitempty"`
	Actor     string   `json:"actor_subject_id,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	Projects  []string `json:"project_ids,omitempty"`
}

func New(ctx context.Context, db store.AuthDatabase, cfg config.Config) (*Service, error) {
	client := &http.Client{
		Timeout:       8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	sessionCipher, err := newCipher(cfg.Server.Auth.SessionKey)
	if err != nil {
		return nil, err
	}
	contextCipher, err := newCipher(cfg.Server.Auth.ContextKey)
	if err != nil {
		return nil, err
	}
	auth := cfg.Server.Auth
	oidcContext := oidc.ClientContext(ctx, client)
	verifier := oidc.NewVerifier(
		auth.Issuer,
		oidc.NewRemoteKeySet(oidcContext, auth.Issuer+"/oauth2/jwks"),
		&oidc.Config{ClientID: auth.ClientID, SupportedSigningAlgs: []string{"RS256"}},
	)
	service := &Service{
		db:            db,
		cfg:           cfg,
		http:          client,
		verifier:      verifier,
		sessionCipher: sessionCipher,
		contextCipher: contextCipher,
		oauth: oauth2.Config{
			ClientID:     auth.ClientID,
			ClientSecret: string(auth.ClientSecret),
			RedirectURL:  auth.PublicBaseURL + "/api/v1/auth/oidc/callback",
			Scopes:       []string{"openid", "profile", "email", "offline_access"},
			Endpoint: oauth2.Endpoint{
				AuthURL:   auth.Issuer + "/oauth2/authorize",
				TokenURL:  auth.Issuer + "/oauth2/token",
				AuthStyle: oauth2.AuthStyleInHeader,
			},
		},
		tokens:         map[string]cachedToken{},
		revocationWake: make(chan struct{}, 1),
	}
	if len(cfg.Gateway.ConfigurationKey) != 0 {
		service.agentDB, _ = db.(store.AgentAccessDatabase)
		if service.agentDB == nil {
			return nil, fmt.Errorf("agent access storage is unavailable")
		}
		service.agentCipher, err = agentaccess.NewCipher(cfg.Gateway.ConfigurationKey)
		if err != nil {
			return nil, err
		}
	}
	return service, nil
}

func newCipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(aead cipher.AEAD, purpose string, value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, raw, []byte(purpose)), nil
}

func open(aead cipher.AEAD, purpose string, encrypted []byte, value any) error {
	if len(encrypted) < aead.NonceSize() {
		return ErrInvalid
	}
	raw, err := aead.Open(nil, encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():], []byte(purpose))
	if err != nil {
		return ErrInvalid
	}
	return json.Unmarshal(raw, value)
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func hash(raw string) []byte           { value := sha256.Sum256([]byte(raw)); return value[:] }
func challenge(verifier string) string { return base64.RawURLEncoding.EncodeToString(hash(verifier)) }
func canonicalRandom(raw string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw
}
func upstreamError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	return ErrUnavailable
}

func (s *Service) InitLogin(ctx context.Context, appChallenge string) (string, error) {
	if !canonicalRandom(appChallenge) {
		return "", ErrInvalid
	}
	verifier, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	nonce, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	state, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	id := uuid.NewString()
	encrypted, err := seal(
		s.contextCipher,
		"ir:oidc:"+id,
		pendingContext{Verifier: verifier, Nonce: nonce, AppChallenge: appChallenge},
	)
	if err != nil {
		return "", ErrUnavailable
	}
	if err := s.db.CreateLoginRequest(ctx, model.LoginRequest{ID: id, Kind: "login", StateHash: hash(state), EncryptedContext: encrypted, ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		return "", ErrUnavailable
	}
	return s.oauth.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

func (s *Service) LoginCallback(ctx context.Context, state, code string) (string, error) {
	if !canonicalRandom(state) || code == "" {
		return "", ErrInvalid
	}
	pending, err := s.db.ConsumeLoginCallback(ctx, hash(state))
	if err != nil {
		return "", upstreamError(err)
	}
	var contextData pendingContext
	if pending.Kind != "login" ||
		open(s.contextCipher, "ir:oidc:"+pending.ID, pending.EncryptedContext, &contextData) != nil {
		return "", ErrInvalid
	}
	oauthToken, err := s.oauth.Exchange(
		context.WithValue(ctx, oauth2.HTTPClient, s.http),
		code,
		oauth2.SetAuthURLParam("code_verifier", contextData.Verifier),
	)
	if err != nil {
		return "", ErrUnauthorized
	}
	id := uuid.NewString()
	tokens := tokenSet{AccessToken: oauthToken.AccessToken, RefreshToken: oauthToken.RefreshToken}
	persisted := false
	defer func() {
		if !persisted && tokens.RefreshToken != "" {
			s.queueRevocation(ctx, id, tokens)
		}
	}()
	rawID, ok := oauthToken.Extra("id_token").(string)
	if !ok || tokens.RefreshToken == "" {
		return "", ErrUnauthorized
	}
	if !validIDTokenHeader(rawID) {
		return "", ErrUnauthorized
	}
	verifiedID, err := s.verifier.Verify(ctx, rawID)
	if err != nil || verifiedID.Nonce != contextData.Nonce || verifiedID.AccessTokenHash == "" ||
		verifiedID.VerifyAccessToken(tokens.AccessToken) != nil {
		return "", ErrUnauthorized
	}
	var profile struct {
		Subject  string `json:"sub"`
		Username string `json:"preferred_username"`
		Email    string `json:"email"`
		AuthTime int64  `json:"auth_time"`
	}
	if verifiedID.Claims(&profile) != nil {
		return "", ErrUnauthorized
	}
	identity, err := s.Verify(ctx, tokens.AccessToken)
	if err != nil || identity.SubjectID != profile.Subject {
		return "", ErrUnauthorized
	}
	active, err := s.introspect(ctx, tokens.AccessToken)
	if err != nil {
		return "", err
	}
	if !active.Active || active.SessionExpiresAt <= time.Now().Unix() || active.Subject != identity.SubjectID ||
		active.SessionID != identity.SessionID ||
		active.ClientID != s.cfg.Server.Auth.ClientID {
		return "", ErrUnauthorized
	}
	handle, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	encrypted, err := seal(s.sessionCipher, "ir:session:"+id, tokens)
	if err != nil {
		return "", ErrUnavailable
	}
	publicProfile, _ := json.Marshal(
		map[string]any{
			"subject_id": identity.SubjectID,
			"kind":       "user",
			"is_active":  true,
			"auth_time":  profile.AuthTime,
			"user": map[string]string{
				"user_id":  identity.SubjectID,
				"username": profile.Username,
				"email":    profile.Email,
			},
		},
	)
	deadline := time.Now().Add(7 * 24 * time.Hour)
	if authDeadline := time.Unix(active.SessionExpiresAt, 0); authDeadline.Before(deadline) {
		deadline = authDeadline
	}
	session := model.LoginSession{
		ID:              id,
		SubjectID:       identity.SubjectID,
		ExternalSID:     identity.SessionID,
		RefreshHash:     hash(handle),
		EncryptedTokens: encrypted,
		Profile:         publicProfile,
		ExpiresAt:       deadline,
	}
	if err := s.db.CreateLoginSession(ctx, session); err != nil {
		return "", ErrUnavailable
	}
	handoff, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	result, err := seal(s.contextCipher, "ir:handoff:"+pending.ID, handoffResult{SessionID: id, RefreshHandle: handle})
	if err != nil {
		return "", ErrUnavailable
	}
	if err := s.db.FinishLoginCallback(ctx, pending.ID, hash(handoff), result); err != nil {
		return "", ErrUnavailable
	}
	persisted = true
	return s.cfg.Server.Auth.WebURL + "/?auth_handoff=" + url.QueryEscape(handoff), nil
}

func (s *Service) Redeem(ctx context.Context, handoff, appVerifier string) (model.LoginSession, string, string, error) {
	if !canonicalRandom(handoff) || !canonicalRandom(appVerifier) {
		return model.LoginSession{}, "", "", ErrInvalid
	}
	pending, err := s.db.RedeemLoginHandoff(ctx, hash(handoff))
	if err != nil {
		return model.LoginSession{}, "", "", upstreamError(err)
	}
	var contextData pendingContext
	var result handoffResult
	if pending.Kind != "login" ||
		open(s.contextCipher, "ir:oidc:"+pending.ID, pending.EncryptedContext, &contextData) != nil ||
		contextData.AppChallenge != challenge(appVerifier) ||
		open(s.contextCipher, "ir:handoff:"+pending.ID, pending.EncryptedResult, &result) != nil {
		return model.LoginSession{}, "", "", ErrInvalid
	}
	session, err := s.db.LoginSessionByID(ctx, result.SessionID)
	if err != nil {
		return session, "", "", upstreamError(err)
	}
	var tokens tokenSet
	if open(s.sessionCipher, "ir:session:"+session.ID, session.EncryptedTokens, &tokens) != nil {
		return session, "", "", ErrUnauthorized
	}
	return session, tokens.AccessToken, result.RefreshHandle, nil
}

func (s *Service) Verify(ctx context.Context, raw string) (coreauth.AccessTokenIdentity, error) {
	id, err := coreauth.VerifyAccessToken(
		ctx,
		raw,
		coreauth.VerificationConfig{
			Issuer:   s.cfg.Server.Auth.Issuer,
			Audience: coreauth.AudienceIR,
			Keys:     s.cfg.Server.Auth.VerificationKeys,
			Policy:   s.cfg.Server.Auth.AccessTokenPolicy,
		},
	)
	if errors.Is(err, coreauth.ErrKeysUnavailable) {
		return id, ErrUnavailable
	}
	if err != nil || id.SubjectKind != coreauth.SubjectUser || id.ClientID != s.cfg.Server.Auth.ClientID ||
		id.ActorSubjectID != "" {
		return id, ErrUnauthorized
	}
	return id, nil
}

func (s *Service) Authenticate(
	ctx context.Context,
	raw string,
) (model.LoginSession, coreauth.AccessTokenIdentity, error) {
	id, err := s.Verify(ctx, raw)
	if err != nil {
		return model.LoginSession{}, id, err
	}
	session, err := s.db.LoginSessionByIdentity(ctx, id.SessionID, id.SubjectID)
	if err != nil {
		return session, id, upstreamError(err)
	}
	return session, id, nil
}

func (s *Service) introspect(ctx context.Context, raw string) (coreauth.IntrospectionResponse, error) {
	var response coreauth.IntrospectionResponse
	_, err := s.authForm(
		ctx,
		"/oauth2/introspect",
		url.Values{"token": {raw}, "token_type_hint": {"access_token"}},
		"",
		&response,
	)
	return response, err
}

func (s *Service) Refresh(ctx context.Context, handle string) (model.LoginSession, string, error) {
	if !canonicalRandom(handle) {
		return model.LoginSession{}, "", ErrUnauthorized
	}
	// A database lease coordinates replicas; HTTP never holds a database lock.
	// A retry after worker death uses the same Auth Idempotency-Key.
	for attempt := 0; attempt < 100; attempt++ {
		session, err := s.db.LoginSessionByRefresh(ctx, hash(handle))
		if err != nil {
			return session, "", upstreamError(err)
		}
		var tokens tokenSet
		if open(s.sessionCipher, "ir:session:"+session.ID, session.EncryptedTokens, &tokens) != nil {
			return session, "", ErrUnauthorized
		}
		id, verifyErr := s.Verify(ctx, tokens.AccessToken)
		if errors.Is(verifyErr, ErrUnavailable) {
			return session, "", verifyErr
		}
		if verifyErr == nil {
			if id.SubjectID != session.SubjectID || id.SessionID != session.ExternalSID {
				return session, "", ErrUnauthorized
			}
			if id.ExpiresAt.After(time.Now().Add(30 * time.Second)) {
				return session, tokens.AccessToken, nil
			}
		}
		operation := uuid.NewString()
		reserved, err := s.db.ReserveLoginRefresh(ctx, session.ID, session.Generation, operation)
		if err != nil {
			return session, "", ErrUnavailable
		}
		if !reserved {
			select {
			case <-ctx.Done():
				return session, "", ErrUnavailable
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		access, err := s.rotate(ctx, session, tokens, operation)
		return session, access, err
	}
	return model.LoginSession{}, "", ErrUnavailable
}

func (s *Service) rotate(
	ctx context.Context,
	session model.LoginSession,
	old tokenSet,
	operation string,
) (string, error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = s.db.ReleaseLoginRefresh(cleanup, session.ID, operation)
	}()
	var next tokenSet
	_, err := s.authForm(
		ctx,
		"/oauth2/token",
		url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old.RefreshToken}},
		hex.EncodeToString(hash(old.RefreshToken)),
		&next,
	)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			_ = s.db.RevokeLoginSession(ctx, session.ID)
		}
		return "", err
	}
	identity, err := s.Verify(ctx, next.AccessToken)
	if errors.Is(err, ErrUnavailable) {
		// Auth rotated successfully; replay the same idempotent operation once
		// verification keys recover instead of revoking the usable session.
		return "", err
	}
	if err != nil || identity.SubjectID != session.SubjectID || identity.SessionID != session.ExternalSID ||
		next.RefreshToken == "" {
		s.queueRevocation(ctx, session.ID, next)
		return "", ErrUnauthorized
	}
	encrypted, err := seal(s.sessionCipher, "ir:session:"+session.ID, next)
	if err != nil {
		return "", ErrUnavailable
	}
	saved, err := s.db.CompleteLoginRefresh(ctx, session.ID, session.Generation, operation, encrypted)
	if err != nil {
		return "", ErrUnavailable
	}
	if !saved {
		current, currentErr := s.db.LoginSessionByID(ctx, session.ID)
		if currentErr != nil {
			if errors.Is(currentErr, pgx.ErrNoRows) {
				s.queueRevocation(ctx, session.ID, next)
			}
			return "", upstreamError(currentErr)
		}
		if current.Generation <= session.Generation {
			return "", ErrUnavailable
		}
		var committed tokenSet
		if open(s.sessionCipher, "ir:session:"+current.ID, current.EncryptedTokens, &committed) != nil {
			return "", ErrUnavailable
		}
		winner, verifyErr := s.Verify(ctx, committed.AccessToken)
		if errors.Is(verifyErr, ErrUnavailable) {
			return "", verifyErr
		}
		if verifyErr != nil || winner.SubjectID != session.SubjectID || winner.SessionID != session.ExternalSID ||
			!winner.ExpiresAt.After(time.Now().Add(30*time.Second)) {
			return "", ErrUnavailable
		}
		return committed.AccessToken, nil
	}
	return next.AccessToken, nil
}

func (s *Service) Logout(ctx context.Context, handle string) error {
	if !canonicalRandom(handle) {
		return nil
	}
	session, err := s.db.LoginSessionByRefresh(ctx, hash(handle))
	if errors.Is(err, pgx.ErrNoRows) {
		previous, previousErr := s.db.PreviousLoginRefresh(ctx, hash(handle))
		if previousErr == nil {
			session = previous.Session
		}
		err = previousErr
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	if err := s.db.RevokeLoginSession(ctx, session.ID); err != nil {
		return ErrUnavailable
	}
	s.cacheMu.Lock()
	s.tokens = map[string]cachedToken{}
	s.cacheMu.Unlock()
	// Try only this session, with a short timeout. The durable worker handles
	// backlog and retries, so one logout cannot wait on other users' revocations.
	var tokens tokenSet
	if open(s.sessionCipher, "ir:session:"+session.ID, session.EncryptedTokens, &tokens) == nil {
		revokeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, _ = s.authForm(
			revokeCtx,
			"/oauth2/revoke",
			url.Values{"token": {tokens.RefreshToken}, "token_type_hint": {"refresh_token"}},
			"",
			nil,
		)
		cancel()
	}
	select {
	case s.revocationWake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Service) queueRevocation(ctx context.Context, sessionID string, tokens tokenSet) {
	encrypted, err := seal(s.sessionCipher, "ir:session:"+sessionID, tokens)
	if err != nil {
		return
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = s.db.QueueAuthRevocation(
		cleanup,
		model.AuthRevocation{ID: uuid.NewString(), SessionID: sessionID, EncryptedTokens: encrypted},
	)
}

func (s *Service) RunRevocations(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		s.retryRevocations(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.revocationWake:
		}
	}
}

func (s *Service) retryRevocations(ctx context.Context) {
	if expired, err := s.db.ExpireAuthRevocations(ctx); err == nil && len(expired) > 0 {
		slog.Warn("auth_revocation_expired", "count", len(expired))
	}
	pending, err := s.db.ClaimAuthRevocations(ctx, authRevocationBatch)
	if err != nil {
		slog.Warn("auth_revocation_claim_failed", "error", err)
		return
	}
	for _, item := range pending {
		if ctx.Err() != nil {
			return
		}
		var tokens tokenSet
		if open(s.sessionCipher, "ir:session:"+item.SessionID, item.EncryptedTokens, &tokens) != nil {
			slog.Warn("auth_revocation_dropped", "session_id", item.SessionID, "reason", "undecryptable")
			_ = s.db.CompleteAuthRevocation(ctx, item.ID)
			continue
		}
		status, err := s.authForm(
			ctx,
			"/oauth2/revoke",
			url.Values{"token": {tokens.RefreshToken}, "token_type_hint": {"refresh_token"}},
			"",
			nil,
		)
		// Auth returns 200 for unknown or already-revoked tokens. A 400/401
		// response can recover after configuration or credential repair.
		if err == nil {
			_ = s.db.CompleteAuthRevocation(ctx, item.ID)
			continue
		}
		slog.Warn("auth_revocation_deferred", "session_id", item.SessionID,
			"status", status, "attempts", item.Attempts)
	}
}

func (s *Service) authForm(
	ctx context.Context,
	path string,
	values url.Values,
	operation string,
	target any,
) (int, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.cfg.Server.Auth.Issuer+path,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return 0, ErrUnavailable
	}
	request.SetBasicAuth(
		url.QueryEscape(s.cfg.Server.Auth.ClientID),
		url.QueryEscape(string(s.cfg.Server.Auth.ClientSecret)),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if operation != "" {
		request.Header.Set("Idempotency-Key", operation)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return 0, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return response.StatusCode, ErrUnavailable
	}
	if response.StatusCode == http.StatusUnauthorized {
		// OAuth backchannels use 401 for invalid_client, not user-session
		// revocation. Preserve the local session while credentials recover.
		return response.StatusCode, ErrUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusBadRequest && path == "/oauth2/token" {
			var problem struct {
				Error string `json:"error"`
			}
			if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&problem) == nil &&
				problem.Error == "invalid_grant" {
				return response.StatusCode, ErrUnauthorized
			}
		}
		return response.StatusCode, ErrForbidden
	}
	if target != nil && json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(target) != nil {
		return response.StatusCode, ErrUnavailable
	}
	return response.StatusCode, nil
}

// The ID-token profile has a separate algorithm and type from access JWT.
func validIDTokenHeader(raw string) bool {
	if len(raw) > 16384 {
		return false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var fields struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(header)))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] || key == "crit" {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
		if key == "b64" && string(value) != "true" {
			return false
		}
	}
	if _, err := decoder.Token(); err != nil || decoder.Decode(new(any)) != io.EOF {
		return false
	}
	return json.Unmarshal(header, &fields) == nil && fields.Algorithm == "RS256" && fields.Type == "JWT" &&
		fields.KeyID != "" &&
		len(fields.KeyID) <= 128
}
