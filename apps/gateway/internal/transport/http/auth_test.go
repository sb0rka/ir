package httptransport

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	"github.com/sb0rka/ir/packages/common/irconfig"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

func TestGatewayConfigurationAuthorizationAndEmptySources(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var authDown, revoked atomic.Bool
	var apiStatus atomic.Int32
	apiStatus.Store(200)
	issuer := ""
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": "fixture", "x": base64.RawURLEncoding.EncodeToString(public)}}})
		case "/oauth2/introspect":
			if authDown.Load() {
				w.WriteHeader(503)
				return
			}
			_ = r.ParseForm()
			claims := &coreauth.AccessTokenClaims{}
			if _, _, err := jwt.NewParser().ParseUnverified(r.Form.Get("token"), claims); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			json.NewEncoder(w).Encode(coreauth.IntrospectionResponse{Active: !revoked.Load(), Subject: claims.Subject, SessionID: claims.SessionID, ClientID: claims.ClientID, Audience: coreauth.AudienceIRGateway, Actor: claims.Actor.SubjectID, DelegationID: claims.DelegationID, DelegationVersion: claims.DelegationVersion})
		case "/projects/aabbccddee/secrets":
			if r.Header.Get("Authorization") == "" {
				t.Error("API token missing")
			}
			w.WriteHeader(int(apiStatus.Load()))
			io.WriteString(w, `{"secrets":[]}`)
		default:
			t.Error("unexpected backchannel", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer fixture.Close()
	issuer = fixture.URL
	keys, err := coreauth.NewJWKSCache(issuer)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Auth: config.AuthConfig{Issuer: issuer, APIBaseURL: fixture.URL, ClientID: coreauth.ClientIRGateway, ClientSecret: []byte("01234567890123456789012345678901"), Keys: keys}}
	h := NewHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), service.New(reg, nil, time.Second, time.Second))
	sub, sid, actor := uuid.NewString(), uuid.NewString(), uuid.NewString()
	claims := func(audience, scope string) coreauth.AccessTokenClaims {
		return coreauth.AccessTokenClaims{SessionID: sid, SubjectKind: coreauth.SubjectUser, ClientID: coreauth.ClientIRWeb, Actor: &coreauth.Actor{SubjectID: actor}, Scope: scope, DelegationID: uuid.NewString(), DelegationVersion: 1, ProjectIDs: []string{"aabbccddee"}, RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: sub, Audience: jwt.ClaimStrings{audience}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), ID: uuid.NewString()}}
	}
	sign := func(claims coreauth.AccessTokenClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = "fixture"
		token.Header["typ"] = coreauth.AccessTokenType
		raw, err := token.SignedString(private)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	gateway := claims(coreauth.AudienceIRGateway, coreauth.ScopeIRGatewayUse)
	api := claims(coreauth.AudienceAPI, coreauth.ScopeSecretsRead+" "+coreauth.ScopeSecretsReveal)
	request := func(g, a coreauth.AccessTokenClaims, project string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/v1/sources", nil)
		r.Header.Set("Authorization", "Bearer "+sign(g))
		r.Header.Set(irconfig.TokenHeader, "Bearer "+sign(a))
		r.Header.Set("X-Project-ID", project)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(gateway, api, "aabbccddee"); w.Code != 200 || w.Body.String() != "{\"items\":[]}\n" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	reordered := api
	reordered.Scope = coreauth.ScopeSecretsReveal + " " + coreauth.ScopeSecretsRead
	if w := request(gateway, reordered, "aabbccddee"); w.Code != 200 {
		t.Fatal("scope order changed authorization", w.Code)
	}
	for _, mutation := range []func(*coreauth.AccessTokenClaims){func(c *coreauth.AccessTokenClaims) { c.Subject = uuid.NewString() }, func(c *coreauth.AccessTokenClaims) { c.SessionID = uuid.NewString() }, func(c *coreauth.AccessTokenClaims) { c.ClientID = coreauth.ClientSOMWeb }, func(c *coreauth.AccessTokenClaims) { c.Actor = &coreauth.Actor{SubjectID: uuid.NewString()} }, func(c *coreauth.AccessTokenClaims) { c.Audience = jwt.ClaimStrings{coreauth.AudienceIRGateway} }, func(c *coreauth.AccessTokenClaims) {
		c.Scope = coreauth.ScopeSecretsRead
	}, func(c *coreauth.AccessTokenClaims) {
		c.Scope = coreauth.ScopeSecretsReveal
	}, func(c *coreauth.AccessTokenClaims) {
		c.Scope += " " + coreauth.ScopeSecretsWrite
	}} {
		bad := api
		mutation(&bad)
		if w := request(gateway, bad, "aabbccddee"); w.Code != 403 {
			t.Fatal("mismatched configuration identity accepted", w.Code)
		}
	}
	if w := request(gateway, api, "1122334455"); w.Code != 403 {
		t.Fatal("foreign project", w.Code)
	}
	wrong := gateway
	wrong.Audience = jwt.ClaimStrings{coreauth.AudienceIR}
	if w := request(wrong, api, "aabbccddee"); w.Code != 401 {
		t.Fatal("wrong Gateway audience", w.Code)
	}
	apiStatus.Store(403)
	if w := request(gateway, api, "aabbccddee"); w.Code != 403 {
		t.Fatal("revoked API grant", w.Code)
	}
	apiStatus.Store(503)
	if w := request(gateway, api, "aabbccddee"); w.Code != 503 {
		t.Fatal("API outage", w.Code)
	}
	apiStatus.Store(200)
	revoked.Store(true)
	if w := request(gateway, api, "aabbccddee"); w.Code != 403 {
		t.Fatal("revoked Gateway grant", w.Code)
	}
	revoked.Store(false)
	authDown.Store(true)
	if w := request(gateway, api, "aabbccddee"); w.Code != 503 {
		t.Fatal("Auth outage", w.Code)
	}
}
