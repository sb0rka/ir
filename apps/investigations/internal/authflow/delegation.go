package authflow

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

func integrationTarget(kind string) (string, []string, error) {
	switch kind {
	case "platform":
		return coreauth.AudienceAPI, []string{coreauth.ScopeProjectsRead, coreauth.ScopeResourcesRead, coreauth.ScopeSecretsRead, coreauth.ScopeSecretsReveal, coreauth.ScopeSecretsCreate, coreauth.ScopeSecretsWrite}, nil
	case "gateway":
		return coreauth.AudienceIRGateway, []string{coreauth.ScopeIRGatewayUse}, nil
	default:
		return "", nil, ErrInvalid
	}
}

func (s *Service) Connect(ctx context.Context, session model.LoginSession, raw, kind string) (string, error) {
	audience, _, err := integrationTarget(kind)
	if err != nil {
		return "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	state, err := randomToken()
	if err != nil {
		return "", ErrUnavailable
	}
	id := uuid.NewString()
	encrypted, err := seal(s.contextCipher, "ir:oidc:"+id, pendingContext{Verifier: verifier})
	if err != nil {
		return "", ErrUnavailable
	}
	if err := s.db.CreateLoginRequest(ctx, model.LoginRequest{ID: id, Kind: kind, SessionID: session.ID, StateHash: hash(state), EncryptedContext: encrypted, ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		return "", ErrUnavailable
	}
	var result struct {
		URL string `json:"authorize_url"`
	}
	_, err = s.authForm(
		ctx,
		"/oauth2/delegations/request",
		url.Values{
			"subject_token":  {raw},
			"audience":       {audience},
			"redirect_uri":   {s.integrationCallback(kind)},
			"state":          {state},
			"code_challenge": {challenge(verifier)},
		},
		"",
		&result,
	)
	if err != nil {
		return "", err
	}
	expected := s.cfg.Server.Auth.Issuer + "/oauth2/delegations/consent?"
	if !strings.HasPrefix(result.URL, expected) {
		return "", ErrUnavailable
	}
	return result.URL, nil
}

func (s *Service) integrationCallback(kind string) string {
	return s.cfg.Server.Auth.PublicBaseURL + "/api/v1/integrations/" + kind + "/callback"
}

func (s *Service) IntegrationCallback(ctx context.Context, kind, state, code string) (string, error) {
	audience, _, err := integrationTarget(kind)
	if err != nil || !canonicalRandom(state) || code == "" {
		return "", ErrInvalid
	}
	pending, err := s.db.ConsumeLoginCallback(ctx, hash(state))
	if err != nil {
		return "", upstreamError(err)
	}
	var contextData pendingContext
	if pending.Kind != kind ||
		open(s.contextCipher, "ir:oidc:"+pending.ID, pending.EncryptedContext, &contextData) != nil {
		return "", ErrInvalid
	}
	if _, err := s.db.LoginSessionByID(ctx, pending.SessionID); err != nil {
		return "", upstreamError(err)
	}
	var grant Grant
	_, err = s.authForm(
		ctx,
		"/oauth2/token",
		url.Values{
			"grant_type":    {delegationGrant},
			"code":          {code},
			"redirect_uri":  {s.integrationCallback(kind)},
			"code_verifier": {contextData.Verifier},
			"audience":      {audience},
		},
		"",
		&grant,
	)
	if err != nil {
		return "", err
	}
	if grant.Audience != audience || grant.ID == "" || grant.Version < 1 {
		return "", ErrForbidden
	}
	s.clearTokenCache()
	return s.cfg.Server.Auth.WebURL + "/?integration=" + url.QueryEscape(kind), nil
}

func (s *Service) GrantStatus(ctx context.Context, raw, kind string) (Grant, error) {
	audience, _, err := integrationTarget(kind)
	if err != nil {
		return Grant{}, err
	}
	var grant Grant
	_, err = s.authForm(
		ctx,
		"/oauth2/delegations/status",
		url.Values{"subject_token": {raw}, "audience": {audience}},
		"",
		&grant,
	)
	if err != nil {
		return grant, err
	}
	if grant.Connected &&
		(grant.Audience != audience || grant.ID == "" || grant.Version < 1 || len(grant.Projects) == 0) {
		return Grant{}, ErrForbidden
	}
	return grant, nil
}

func (s *Service) Disconnect(ctx context.Context, raw, kind string) error {
	audience, _, err := integrationTarget(kind)
	if err != nil {
		return err
	}
	_, err = s.authForm(
		ctx,
		"/oauth2/delegations/revoke",
		url.Values{"subject_token": {raw}, "audience": {audience}},
		"",
		nil,
	)
	s.clearTokenCache()
	return err
}

func (s *Service) clearTokenCache() {
	s.cacheMu.Lock()
	s.tokens = map[string]cachedToken{}
	s.cacheMu.Unlock()
}

func sameStrings(left, right []string) bool {
	a, b := slices.Clone(left), slices.Clone(right)
	sort.Strings(a)
	sort.Strings(b)
	return slices.Equal(a, b)
}

func (s *Service) DelegatedToken(
	ctx context.Context,
	session model.LoginSession,
	raw, kind string,
	scopes []string,
	project string,
) (string, error) {
	audience, allowed, err := integrationTarget(kind)
	if err != nil {
		return "", err
	}
	grant, err := s.GrantStatus(ctx, raw, kind)
	if err != nil {
		return "", err
	}
	if !grant.Connected {
		return "", ErrForbidden
	}
	if project != "" && !slices.Contains(grant.Projects, project) {
		return "", ErrNotFound
	}
	for _, scope := range scopes {
		if !slices.Contains(allowed, scope) || !slices.Contains(grant.Scopes, scope) {
			return "", ErrForbidden
		}
	}
	keyData, _ := json.Marshal([]any{session.ID, audience, grant.ID, grant.Version, scopes, grant.Projects})
	key := hex.EncodeToString(hash(string(keyData)))
	s.cacheMu.Lock()
	cached, ok := s.tokens[key]
	s.cacheMu.Unlock()
	if ok && cached.Expires.After(time.Now().Add(10*time.Second)) {
		return cached.Raw, nil
	}
	var issued tokenSet
	_, err = s.authForm(
		ctx,
		"/oauth2/token",
		url.Values{
			"grant_type":           {exchangeGrant},
			"subject_token":        {raw},
			"subject_token_type":   {tokenTypeURN},
			"requested_token_type": {tokenTypeURN},
			"audience":             {audience},
			"delegation_id":        {grant.ID},
			"scope":                {strings.Join(scopes, " ")},
		},
		"",
		&issued,
	)
	if err != nil {
		return "", err
	}
	id, err := coreauth.VerifyAccessToken(
		ctx,
		issued.AccessToken,
		coreauth.VerificationConfig{
			Issuer:   s.cfg.Server.Auth.Issuer,
			Audience: audience,
			Keys:     s.cfg.Server.Auth.VerificationKeys,
			Policy:   s.cfg.Server.Auth.AccessTokenPolicy,
		},
	)
	if err != nil || id.SubjectID != session.SubjectID || id.SessionID != session.ExternalSID ||
		id.ClientID != s.cfg.Server.Auth.ClientID ||
		id.ActorSubjectID != grant.Actor ||
		id.DelegationID != grant.ID ||
		id.DelegationVersion != grant.Version ||
		!sameStrings(strings.Fields(id.Scope), scopes) ||
		!sameStrings(id.ProjectIDs, grant.Projects) ||
		issued.RefreshToken != "" {
		return "", ErrForbidden
	}
	s.cacheMu.Lock()
	if len(s.tokens) >= 1024 {
		s.tokens = map[string]cachedToken{}
	}
	s.tokens[key] = cachedToken{Raw: issued.AccessToken, Expires: id.ExpiresAt}
	s.cacheMu.Unlock()
	return issued.AccessToken, nil
}

func (s *Service) CheckProject(ctx context.Context, session model.LoginSession, raw, project string) error {
	delegated, err := s.DelegatedToken(ctx, session, raw, "platform", []string{coreauth.ScopeProjectsRead}, project)
	if err != nil {
		return err
	}
	payload, err := s.platformGET(ctx, delegated, "/projects/"+url.PathEscape(project))
	if err != nil {
		return err
	}
	var current struct {
		PublicID string `json:"public_id"`
		Active   bool   `json:"is_active"`
	}
	if json.Unmarshal(payload, &current) != nil {
		return ErrUnavailable
	}
	if current.PublicID != project || !current.Active {
		return ErrNotFound
	}
	return nil
}

// Projects uses the delegated API response, so filtering and user RBAC remain
// enforced by API even when a client-side project selection is stale.
func (s *Service) Projects(ctx context.Context, session model.LoginSession, raw string) (json.RawMessage, error) {
	delegated, err := s.DelegatedToken(ctx, session, raw, "platform", []string{coreauth.ScopeProjectsRead}, "")
	if err != nil {
		return nil, err
	}
	return s.platformGET(ctx, delegated, "/projects")
}

func (s *Service) platformGET(ctx context.Context, token, path string) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.Platform.APIBaseURL+path, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := s.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusForbidden, http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusOK:
	default:
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil || !json.Valid(raw) {
		return nil, ErrUnavailable
	}
	return raw, nil
}

func (s *Service) GatewayBaseURL() string { return s.cfg.Gateway.BaseURL }

func (s *Service) GatewayToken(ctx context.Context, raw, project string) (string, error) {
	if IsAgentBearer(raw) {
		if _, err := s.VerifyAgent(ctx, raw, project); err != nil {
			return "", err
		}
		return raw, nil
	}
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return "", err
	}
	if err := s.CheckProject(ctx, session, raw, project); err != nil {
		return "", err
	}
	return s.DelegatedToken(ctx, session, raw, "gateway", []string{coreauth.ScopeIRGatewayUse}, project)
}
