package authflow

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

type handleStore struct {
	store.AuthDatabase
	mu           sync.Mutex
	session      model.LoginSession
	previousHash []byte
	previous     model.PreviousRefresh
	rotations    int
}

func (d *handleStore) LoginSessionByRefresh(_ context.Context, hash []byte) (model.LoginSession, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session.Revoked || !bytes.Equal(hash, d.session.RefreshHash) {
		return model.LoginSession{}, pgx.ErrNoRows
	}
	return d.session, nil
}
func (d *handleStore) PreviousLoginRefresh(_ context.Context, hash []byte) (model.PreviousRefresh, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session.Revoked || !bytes.Equal(hash, d.previousHash) {
		return model.PreviousRefresh{}, pgx.ErrNoRows
	}
	p := d.previous
	p.Session = d.session
	return p, nil
}
func (d *handleStore) RotateLoginHandle(_ context.Context, id string, current, next, encrypted []byte) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session.Revoked || d.session.ID != id || !bytes.Equal(current, d.session.RefreshHash) {
		return false, nil
	}
	d.previousHash = current
	d.session.RefreshHash = next
	d.previous = model.PreviousRefresh{EncryptedHandle: encrypted, RotatedAt: time.Now()}
	d.rotations++
	return true, nil
}
func (d *handleStore) RevokeLoginSession(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id == d.session.ID {
		d.session.Revoked = true
	}
	return nil
}

func handleFixture(t *testing.T) (*Service, *handleStore, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/jwks" {
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": "fixture", "x": base64.RawURLEncoding.EncodeToString(public)}}})
			return
		}
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(fixture.Close)
	keys, err := coreauth.NewJWKSCache(fixture.URL)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := newCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	session := model.LoginSession{ID: uuid.NewString(), SubjectID: uuid.NewString(), ExternalSID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, coreauth.AccessTokenClaims{SessionID: session.ExternalSID, SubjectKind: coreauth.SubjectUser, ClientID: coreauth.ClientIRWeb, RegisteredClaims: jwt.RegisteredClaims{Issuer: fixture.URL, Subject: session.SubjectID, Audience: jwt.ClaimStrings{coreauth.AudienceIR}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), ID: uuid.NewString()}})
	token.Header["kid"] = "fixture"
	token.Header["typ"] = coreauth.AccessTokenType
	raw, err := token.SignedString(private)
	if err != nil {
		t.Fatal(err)
	}
	session.EncryptedTokens, err = seal(aead, "ir:session:"+session.ID, tokenSet{AccessToken: raw, RefreshToken: "fixture-auth-refresh"})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	session.RefreshHash = hash(handle)
	db := &handleStore{session: session}
	s := &Service{db: db, http: fixture.Client(), sessionCipher: aead, revocationWake: make(chan struct{}, 1), tokens: map[string]cachedToken{}, cfg: config.Config{Server: config.ServerConfig{Auth: config.AuthConfig{Issuer: fixture.URL, ClientID: coreauth.ClientIRWeb, VerificationKeys: keys}}}}
	return s, db, handle
}

func TestLocalRefreshConcurrentReplayAndRevocation(t *testing.T) {
	s, db, old := handleFixture(t)
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, access, next, err := s.RefreshRotating(context.Background(), old)
			if err != nil || access == "" {
				t.Error("refresh failed", err)
				return
			}
			results <- next
		}()
	}
	wg.Wait()
	close(results)
	current := ""
	count := 0
	for next := range results {
		count++
		if current != "" && current != next {
			t.Fatal("concurrent refresh returned different handles")
		}
		current = next
	}
	if count != 8 || current == old || db.rotations != 1 {
		t.Fatal("refresh did not make exactly one transition", count, db.rotations)
	}
	db.mu.Lock()
	db.previous.RotatedAt = time.Now().Add(-31 * time.Second)
	db.mu.Unlock()
	if _, _, _, err := s.RefreshRotating(context.Background(), old); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale handle did not fail", err)
	}
	if _, _, _, err := s.RefreshRotating(context.Background(), current); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("replayed family remained active", err)
	}
	if !db.session.Revoked {
		t.Fatal("session not revoked")
	}
}

func TestLogoutAcceptsPreviousHandleAndCannotRefresh(t *testing.T) {
	s, db, old := handleFixture(t)
	_, _, current, err := s.RefreshRotating(context.Background(), old)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if !db.session.Revoked {
		t.Fatal("logout did not revoke rotated handle")
	}
	if _, _, _, err := s.RefreshRotating(context.Background(), current); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logout session resurrected", err)
	}
}

func TestIDTokenHeaderProfile(t *testing.T) {
	for _, tc := range []struct {
		header string
		valid  bool
	}{
		{`{"alg":"RS256","typ":"JWT","kid":"key"}`, true},
		{`{"alg":"RS256","typ":"JWT","kid":"key","b64":true}`, true},
		{`{"alg":"RS256","typ":"JWT","kid":"key","crit":[]}`, false},
		{`{"alg":"RS256","typ":"JWT","kid":"key","b64":false}`, false},
		{`{"alg":"RS256","typ":"JWT","kid":"key","kid":"other"}`, false},
		{`{"alg":"EdDSA","typ":"JWT","kid":"key"}`, false},
	} {
		raw := base64.RawURLEncoding.EncodeToString([]byte(tc.header)) + ".payload.signature"
		if validIDTokenHeader(raw) != tc.valid {
			t.Fatal("unexpected header validation", tc.header)
		}
	}
}
