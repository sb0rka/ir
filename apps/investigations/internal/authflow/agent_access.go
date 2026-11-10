package authflow

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/agentaccess"
	"github.com/sb0rka/ir/packages/common/irconfig"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

type AgentAccessMetadata struct {
	Configured bool       `json:"configured"`
	SecretID   string     `json:"secret_id,omitempty"`
	Version    int32      `json:"version,omitempty"`
	Prefix     string     `json:"prefix,omitempty"`
	Postfix    string     `json:"postfix,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type AgentAccessIssued struct {
	AgentAccessMetadata
	Token string `json:"token"`
}

func agentMetadata(record agentaccess.Record) AgentAccessMetadata {
	return AgentAccessMetadata{Configured: true, SecretID: record.SecretID, Version: record.VersionNo,
		Prefix: record.Prefix, Postfix: record.Postfix, CreatedAt: &record.CreatedAt, UpdatedAt: &record.UpdatedAt}
}

func IsAgentBearer(raw string) bool { return strings.HasPrefix(raw, "s0_ir_agent_") }

func (s *Service) VerifyAgent(ctx context.Context, raw, project string) (agentaccess.Record, error) {
	if !agentaccess.IsToken(raw) {
		return agentaccess.Record{}, ErrUnauthorized
	}
	if s.agentDB == nil {
		return agentaccess.Record{}, ErrUnavailable
	}
	record, err := s.agentDB.Verify(ctx, project, raw)
	if errors.Is(err, agentaccess.ErrUnauthorized) || errors.Is(err, agentaccess.ErrNotFound) {
		return record, ErrUnauthorized
	}
	if err != nil {
		return record, ErrUnavailable
	}
	return record, nil
}

func (s *Service) AuthenticateProjectRequest(ctx context.Context, raw, project string) (coreauth.AccessTokenIdentity, error) {
	if IsAgentBearer(raw) {
		record, err := s.VerifyAgent(ctx, raw, project)
		if err != nil {
			return coreauth.AccessTokenIdentity{}, err
		}
		return coreauth.AccessTokenIdentity{SubjectID: record.SecretID, SubjectKind: coreauth.SubjectAgent}, nil
	}
	session, identity, err := s.Authenticate(ctx, raw)
	if err != nil {
		return identity, err
	}
	return identity, s.CheckProject(ctx, session, raw, project)
}

func (s *Service) AgentAccess(ctx context.Context, raw, project string) (AgentAccessMetadata, error) {
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return AgentAccessMetadata{}, err
	}
	if err = s.CheckProject(ctx, session, raw, project); err != nil {
		return AgentAccessMetadata{}, err
	}
	if s.agentDB == nil || s.agentCipher == nil {
		return AgentAccessMetadata{}, ErrUnavailable
	}
	record, err := s.agentDB.AgentSecret(ctx, project)
	if errors.Is(err, agentaccess.ErrNotFound) {
		return AgentAccessMetadata{}, nil
	}
	if err != nil {
		return AgentAccessMetadata{}, ErrUnavailable
	}
	return agentMetadata(record), nil
}

func (s *Service) IssueAgentAccess(ctx context.Context, raw, project string, expectedVersion int32) (AgentAccessIssued, error) {
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return AgentAccessIssued{}, err
	}
	if err = s.CheckProject(ctx, session, raw, project); err != nil {
		return AgentAccessIssued{}, err
	}
	if expectedVersion < 0 {
		return AgentAccessIssued{}, ErrInvalid
	}
	if s.agentDB == nil || s.agentCipher == nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	release, err := s.agentDB.Lock(ctx, project)
	if err != nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	defer release()
	current, err := s.agentDB.AgentSecret(ctx, project)
	if err != nil && !errors.Is(err, agentaccess.ErrNotFound) {
		return AgentAccessIssued{}, ErrUnavailable
	}
	if current.VersionNo != expectedVersion {
		return AgentAccessIssued{}, ErrConflict
	}
	platformToken, err := s.DelegatedToken(ctx, session, raw, "platform", []string{
		coreauth.ScopeSecretsRead, coreauth.ScopeSecretsReveal, coreauth.ScopeSecretsCreate, coreauth.ScopeSecretsWrite,
	}, project)
	if err != nil {
		return AgentAccessIssued{}, err
	}
	configurationClient, err := common.NewConfigurationClient(s.cfg.Platform.APIBaseURL)
	if err != nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	document, err := configurationClient.Read(ctx, platformToken, project)
	if err != nil {
		return AgentAccessIssued{}, configurationError(err)
	}
	encrypted, err := s.agentCipher.Seal(project, document)
	if err != nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	token, err := agentaccess.Generate()
	if err != nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	secrets, err := common.NewAgentSecretClient(s.cfg.Platform.APIBaseURL)
	if err != nil {
		return AgentAccessIssued{}, ErrUnavailable
	}
	ref, err := secrets.Write(ctx, platformToken, project, current.SecretID, token)
	if err != nil {
		if errors.Is(err, agentaccess.ErrConflict) {
			return AgentAccessIssued{}, ErrConflict
		}
		return AgentAccessIssued{}, configurationError(err)
	}
	digest := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	record := agentaccess.Record{ProjectID: project, SecretID: ref.SecretID, VersionNo: ref.VersionNo,
		DigestAlg: "sha256", Digest: digest[:], Prefix: token[:14], Postfix: token[len(token)-3:],
		CreatedAt: ref.CreatedAt, UpdatedAt: now}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if !current.CreatedAt.IsZero() {
		record.CreatedAt = current.CreatedAt
	}
	if err = s.agentDB.SaveAgent(ctx, record, encrypted, expectedVersion); err != nil {
		if errors.Is(err, agentaccess.ErrConflict) {
			return AgentAccessIssued{}, ErrConflict
		}
		return AgentAccessIssued{}, ErrUnavailable
	}
	return AgentAccessIssued{AgentAccessMetadata: agentMetadata(record), Token: token}, nil
}

func (s *Service) syncGatewayConfiguration(ctx context.Context, project string, document irconfig.Document) error {
	if s.agentCipher == nil || s.agentDB == nil {
		return nil
	}
	configuration, err := s.agentCipher.Seal(project, document)
	if err != nil {
		return ErrUnavailable
	}
	if err = s.agentDB.PutConfiguration(ctx, configuration); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) lockGatewayConfiguration(ctx context.Context, project string) (func(), error) {
	if s.agentDB == nil || s.agentCipher == nil {
		return func() {}, nil
	}
	release, err := s.agentDB.Lock(ctx, project)
	if err != nil {
		return nil, ErrUnavailable
	}
	return release, nil
}
