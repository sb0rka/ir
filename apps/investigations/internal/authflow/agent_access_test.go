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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

const agentAccessProject = "aabbccddee"

type agentAccessStore struct {
	store.AuthDatabase
	store.AgentAccessDatabase
	session       model.LoginSession
	record        agentaccess.Record
	configuration agentaccess.Configuration
	saveError     error
	verifyError   error
	verifyCalls   int
	saveCalls     int
	locks         int
	releases      int
}

func (d *agentAccessStore) LoginSessionByIdentity(_ context.Context, sid, subject string) (model.LoginSession, error) {
	if d.session.Revoked || sid != d.session.ExternalSID || subject != d.session.SubjectID {
		return model.LoginSession{}, pgx.ErrNoRows
	}
	return d.session, nil
}

func (d *agentAccessStore) AgentSecret(_ context.Context, project string) (agentaccess.Record, error) {
	if d.record.VersionNo == 0 || d.record.ProjectID != project {
		return agentaccess.Record{}, agentaccess.ErrNotFound
	}
	return d.record, nil
}

func (d *agentAccessStore) Verify(ctx context.Context, project, raw string) (agentaccess.Record, error) {
	d.verifyCalls++
	if d.verifyError != nil {
		return agentaccess.Record{}, d.verifyError
	}
	record, err := d.AgentSecret(ctx, project)
	if err != nil {
		return record, err
	}
	if !agentaccess.Matches(raw, record.Digest) {
		return agentaccess.Record{}, agentaccess.ErrUnauthorized
	}
	return record, nil
}

func (d *agentAccessStore) SaveAgent(_ context.Context, record agentaccess.Record, configuration agentaccess.Configuration, expected int32) error {
	d.saveCalls++
	if d.saveError != nil {
		return d.saveError
	}
	if d.record.VersionNo != expected {
		return agentaccess.ErrConflict
	}
	d.record, d.configuration = record, configuration
	return nil
}

func (d *agentAccessStore) Lock(_ context.Context, _ string) (func(), error) {
	d.locks++
	return func() { d.releases++ }, nil
}

type agentAccessRemote struct {
	mu               sync.Mutex
	version          int32
	token            string
	writes           int
	denySecretWrites bool
}

func (r *agentAccessRemote) snapshot() (int32, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version, r.token, r.writes
}

func agentAccessFixture(t *testing.T) (*Service, *agentAccessStore, *agentAccessRemote, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	db := &agentAccessStore{session: model.LoginSession{ID: uuid.NewString(), SubjectID: uuid.NewString(), ExternalSID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}}
	remote := &agentAccessRemote{}
	agentID, configurationID, grantID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	scopes := []string{coreauth.ScopeProjectsRead, coreauth.ScopeResourcesRead, coreauth.ScopeSecretsRead, coreauth.ScopeSecretsReveal, coreauth.ScopeSecretsCreate, coreauth.ScopeSecretsWrite}
	var issuer string
	sign := func(audience, scope string) string {
		claims := coreauth.AccessTokenClaims{SessionID: db.session.ExternalSID, SubjectKind: coreauth.SubjectUser, ClientID: coreauth.ClientIRWeb,
			RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: db.session.SubjectID, Audience: jwt.ClaimStrings{audience},
				IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), ID: uuid.NewString()}}
		if audience == coreauth.AudienceAPI {
			claims.Actor = &coreauth.Actor{SubjectID: actorID}
			claims.Scope, claims.DelegationID, claims.DelegationVersion, claims.ProjectIDs = scope, grantID, 1, []string{agentAccessProject}
		}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"], token.Header["typ"] = "agent-access-fixture", coreauth.AccessTokenType
		raw, err := token.SignedString(private)
		if err != nil {
			t.Error(err)
		}
		return raw
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /oauth2/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": "agent-access-fixture", "x": base64.RawURLEncoding.EncodeToString(public)}}})
		case "POST /oauth2/delegations/status":
			_ = json.NewEncoder(w).Encode(Grant{Connected: true, ID: grantID, Version: 1, Audience: coreauth.AudienceAPI, Actor: actorID, Scopes: scopes, Projects: []string{agentAccessProject}})
		case "POST /oauth2/token":
			if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != exchangeGrant || r.Form.Get("audience") != coreauth.AudienceAPI {
				t.Error("unexpected token request")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(tokenSet{AccessToken: sign(coreauth.AudienceAPI, r.Form.Get("scope"))})
		case "GET /projects/" + agentAccessProject:
			_ = json.NewEncoder(w).Encode(map[string]any{"public_id": agentAccessProject, "is_active": true})
		case "GET /projects/" + agentAccessProject + "/secrets":
			version, _, _ := remote.snapshot()
			items := []map[string]any{{"public_id": "012345abcdef", "secret_id": configurationID, "name": "IR/SOURCE_CONFIGURATION", "payload_kind": "json", "current_version_no": 2, "updated_at": time.Now().UTC()}}
			if version > 0 {
				items = append(items, map[string]any{"public_id": "fedcba543210", "secret_id": agentID, "name": "IR/AGENT_ACCESS", "payload_kind": "text", "current_version_no": version, "updated_at": time.Now().UTC()})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": items})
		case "POST /projects/" + agentAccessProject + "/resources/012345abcdef/secret/versions/2/reveal":
			_ = json.NewEncoder(w).Encode(map[string]any{"secret_id": configurationID, "version_no": 2, "value": `[{"name":"pt-nad","sessionid":"source-private-credential"}]`})
		case "POST /projects/" + agentAccessProject + "/secret", "POST /projects/" + agentAccessProject + "/resources/fedcba543210/secret/versions":
			var body struct {
				Name  string `json:"name"`
				Kind  string `json:"payload_kind"`
				Value string `json:"secret_value"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !agentaccess.IsToken(body.Value) {
				t.Error("invalid agent secret payload")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			created := strings.HasSuffix(r.URL.Path, "/secret")
			if created && (body.Name != "IR/AGENT_ACCESS" || body.Kind != "text") {
				t.Error("agent key is not an ordinary text secret")
			}
			remote.mu.Lock()
			if remote.denySecretWrites {
				remote.mu.Unlock()
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"permission_denied"}`))
				return
			}
			remote.version++
			remote.token = body.Value
			remote.writes++
			version := remote.version
			remote.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			if created {
				_ = json.NewEncoder(w).Encode(map[string]any{"public_id": "fedcba543210", "secret_id": agentID, "current_version_no": version, "updated_at": time.Now().UTC()})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"version_no": version, "created_at": time.Now().UTC()})
			}
		case "POST /projects/" + agentAccessProject + "/resources/fedcba543210/tag":
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Error("unexpected Auth/Platform call", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fixture.Close)
	issuer = fixture.URL
	keys, err := coreauth.NewJWKSCache(issuer)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := agentaccess.NewCipher(bytes.Repeat([]byte{41}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{db: db, agentDB: db, agentCipher: cipher, http: fixture.Client(), tokens: map[string]cachedToken{},
		cfg: config.Config{Server: config.ServerConfig{Auth: config.AuthConfig{Issuer: issuer, ClientID: coreauth.ClientIRWeb, VerificationKeys: keys}}, Platform: config.PlatformConfig{APIBaseURL: issuer}}}
	return s, db, remote, sign(coreauth.AudienceIR, "")
}

func TestAgentRequestAuthorizationDoesNotNeedUserSessionOrAuth(t *testing.T) {
	token, err := agentaccess.Generate()
	if err != nil {
		t.Fatal(err)
	}
	db := &agentAccessStore{record: agentaccess.Record{ProjectID: agentAccessProject, SecretID: uuid.NewString(), VersionNo: 1, DigestAlg: "sha256", Digest: agentaccess.Digest(token)}}
	s := &Service{agentDB: db}
	ctx := context.Background()
	id, err := s.AuthenticateProjectRequest(ctx, token, agentAccessProject)
	if err != nil || id.SubjectKind != coreauth.SubjectAgent || id.SubjectID != db.record.SecretID {
		t.Fatal("agent did not authenticate independently", err)
	}
	if forwarded, err := s.GatewayToken(ctx, token, agentAccessProject); err != nil || forwarded != token {
		t.Fatal("agent bearer was not passed through", err)
	}
	if configToken, err := s.GatewayConfigToken(ctx, token, agentAccessProject); err != nil || configToken != "" {
		t.Fatal("agent request required a delegated configuration token", err)
	}
	if db.verifyCalls != 3 {
		t.Fatal("agent paths did not verify their key", db.verifyCalls)
	}
	if _, err := s.VerifyAgent(ctx, token, "1122334455"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("key crossed project boundary", err)
	}
	if _, err := s.VerifyAgent(ctx, token[:63], agentAccessProject); !errors.Is(err, ErrUnauthorized) || db.verifyCalls != 4 {
		t.Fatal("malformed token reached storage", err)
	}
	db.verifyError = errors.New("database unavailable")
	if _, err := s.VerifyAgent(ctx, token, agentAccessProject); !errors.Is(err, ErrUnavailable) {
		t.Fatal("database failure was reported as bad credentials", err)
	}
	if _, err := (&Service{}).VerifyAgent(ctx, token, agentAccessProject); !errors.Is(err, ErrUnavailable) {
		t.Fatal("disabled storage did not fail closed", err)
	}
}

func TestAgentIssuanceRotationAndStaleVersion(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	ctx := context.Background()
	metadata, err := s.AgentAccess(ctx, user, agentAccessProject)
	if err != nil || metadata.Configured {
		t.Fatal("absent agent key metadata failed", err)
	}
	first, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil || !agentaccess.IsToken(first.Token) || first.Version != 1 || !first.Configured {
		t.Fatal("initial issue failed", err)
	}
	version, value, writes := remote.snapshot()
	if version != 1 || value != first.Token || writes != 1 || !agentaccess.Matches(first.Token, db.record.Digest) || db.record.SecretID != first.SecretID {
		t.Fatal("remote secret and active digest differ")
	}
	document, err := s.agentCipher.Open(db.configuration)
	if err != nil || document.Values["pt-nad"]["sessionid"] != "source-private-credential" || bytes.Contains(db.configuration.EncryptedConfig, []byte("source-private-credential")) {
		t.Fatal("source configuration was not activated encrypted", err)
	}
	metadata, err = s.AgentAccess(ctx, user, agentAccessProject)
	if err != nil || metadata.Prefix != first.Token[:14] || metadata.Postfix != first.Token[61:] || metadata.Version != 1 {
		t.Fatal("active metadata failed", err)
	}
	if _, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate creation was accepted", err)
	}
	second, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 1)
	if err != nil || second.Version != 2 || second.Token == first.Token {
		t.Fatal("rotation failed", err)
	}
	if _, err := s.VerifyAgent(ctx, first.Token, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old key survived rotation", err)
	}
	if _, err := s.VerifyAgent(ctx, second.Token, agentAccessProject); err != nil {
		t.Fatal("new key did not authenticate", err)
	}
	if _, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale rotation was accepted", err)
	}
	_, _, writes = remote.snapshot()
	if writes != 2 || db.saveCalls != 2 || db.locks != db.releases {
		t.Fatal("conflict wrote a secret or project lock leaked", writes, db.saveCalls, db.locks, db.releases)
	}
}

func TestAgentRotationRetryAfterRemoteWriteAndLocalFailure(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	ctx := context.Background()
	first, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	db.saveError = errors.New("local transaction failed")
	failed, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 1)
	if !errors.Is(err, ErrUnavailable) || failed.Token != "" {
		t.Fatal("failed activation returned credentials", err)
	}
	version, pendingToken, writes := remote.snapshot()
	if version != 2 || writes != 2 || db.record.VersionNo != 1 {
		t.Fatal("failure did not preserve local activation boundary")
	}
	if _, err := s.VerifyAgent(ctx, first.Token, agentAccessProject); err != nil {
		t.Fatal("previous key stopped working after failed activation", err)
	}
	if _, err := s.VerifyAgent(ctx, pendingToken, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("remote-only key was activated", err)
	}
	db.saveError = nil
	retried, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 1)
	if err != nil || retried.Version != 3 || retried.Token == pendingToken {
		t.Fatal("retry did not create and activate a new version", err)
	}
	if _, err := s.VerifyAgent(ctx, first.Token, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("previous key survived completed retry", err)
	}
	if _, err := s.VerifyAgent(ctx, retried.Token, agentAccessProject); err != nil {
		t.Fatal("retried key did not authenticate", err)
	}
	if db.locks != db.releases {
		t.Fatal("failure leaked a project lock")
	}
}

func TestAgentCreationRetryRepairsRemoteOnlySecret(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	ctx := context.Background()
	db.saveError = errors.New("local transaction failed")
	failed, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if !errors.Is(err, ErrUnavailable) || failed.Token != "" || db.record.VersionNo != 0 {
		t.Fatal("failed creation was activated or exposed", err)
	}
	version, pending, writes := remote.snapshot()
	if version != 1 || writes != 1 {
		t.Fatal("remote secret was not created before the local failure")
	}
	if _, err := s.VerifyAgent(ctx, pending, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("remote-only creation was accepted", err)
	}
	db.saveError = nil
	issued, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil || issued.Version != 2 || issued.Token == pending {
		t.Fatal("creation retry did not repair the existing remote secret", err)
	}
	if _, err := s.VerifyAgent(ctx, issued.Token, agentAccessProject); err != nil {
		t.Fatal("retried creation did not authenticate", err)
	}
}

func TestAgentManagementRejectsForeignProjectsAndAgentKeys(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	ctx := context.Background()
	if _, err := s.IssueAgentAccess(ctx, user, "1122334455", 0); !errors.Is(err, ErrNotFound) {
		t.Fatal("issuance crossed the user project grant", err)
	}
	_, _, writes := remote.snapshot()
	if writes != 0 || db.locks != 0 {
		t.Fatal("foreign project request reached issuance")
	}
	issued, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentAccess(ctx, issued.Token, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("agent key read management metadata", err)
	}
	if _, err := s.IssueAgentAccess(ctx, issued.Token, agentAccessProject, 1); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("agent key rotated itself", err)
	}
	_, _, writes = remote.snapshot()
	if writes != 1 {
		t.Fatal("agent management attempt changed the remote secret")
	}
}

func TestLostAgentIssuanceResponseRequiresRotationToShowNewToken(t *testing.T) {
	s, db, _, user := agentAccessFixture(t)
	ctx := context.Background()
	issued, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldToken := issued.Token
	metadata, err := s.AgentAccess(ctx, user, agentAccessProject)
	if err != nil || metadata.Version != issued.Version || metadata.SecretID != issued.SecretID {
		t.Fatal("lost response hid the committed metadata", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil || bytes.Contains(encoded, []byte(oldToken)) || bytes.Contains(encoded, []byte(`"token"`)) || bytes.Contains(encoded, []byte(`"digest"`)) {
		t.Fatal("metadata recovered secret material", err)
	}
	if _, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("replayed creation returned credentials", err)
	}
	rotated, err := s.IssueAgentAccess(ctx, user, agentAccessProject, metadata.Version)
	if err != nil || rotated.Version != 2 || rotated.Token == oldToken {
		t.Fatal("metadata could not be used to rotate the lost token", err)
	}
	if _, err := s.VerifyAgent(ctx, oldToken, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("lost key survived rotation", err)
	}
	if !agentaccess.Matches(rotated.Token, db.record.Digest) {
		t.Fatal("new token was not the activated token")
	}
}

func TestAgentIssuanceRequiresPlatformSecretWritePermission(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	remote.mu.Lock()
	remote.denySecretWrites = true
	remote.mu.Unlock()
	issued, err := s.IssueAgentAccess(context.Background(), user, agentAccessProject, 0)
	if !errors.Is(err, ErrForbidden) || issued.Token != "" {
		t.Fatal("Platform write denial was ignored or exposed a token", err)
	}
	version, _, writes := remote.snapshot()
	if version != 0 || writes != 0 || db.saveCalls != 0 || db.record.VersionNo != 0 {
		t.Fatal("write denial activated local access")
	}
	if db.locks != db.releases {
		t.Fatal("permission denial leaked project lock")
	}
}

func TestAgentActivationConflictDoesNotExposeWrittenToken(t *testing.T) {
	s, db, remote, user := agentAccessFixture(t)
	ctx := context.Background()
	first, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 0)
	if err != nil {
		t.Fatal(err)
	}
	db.saveError = agentaccess.ErrConflict
	issued, err := s.IssueAgentAccess(ctx, user, agentAccessProject, 1)
	if !errors.Is(err, ErrConflict) || issued.Token != "" {
		t.Fatal("activation conflict was not returned without credentials", err)
	}
	version, pending, writes := remote.snapshot()
	if version != 2 || writes != 2 || db.record.VersionNo != 1 {
		t.Fatal("activation conflict changed the local active version")
	}
	if _, err := s.VerifyAgent(ctx, first.Token, agentAccessProject); err != nil {
		t.Fatal("activation conflict invalidated the old token", err)
	}
	if _, err := s.VerifyAgent(ctx, pending, agentAccessProject); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("conflicting remote token was accepted", err)
	}
}
