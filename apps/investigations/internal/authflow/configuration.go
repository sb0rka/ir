package authflow

import (
	"context"
	"errors"
	"net/http"

	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/irconfig"
	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
)

var ErrConflict = errors.New("configuration version conflict")

func configurationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, common.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, common.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, common.ErrSecretNotFound):
		return ErrNotFound
	}
	var response *common.HTTPError
	if errors.As(err, &response) {
		switch response.StatusCode {
		case http.StatusConflict:
			return ErrConflict
		case http.StatusBadRequest:
			return ErrInvalid
		}
	}
	return ErrUnavailable
}

func (s *Service) Configuration(ctx context.Context, raw, project string) (irconfig.Document, error) {
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return irconfig.Document{}, err
	}
	if err = s.CheckProject(ctx, session, raw, project); err != nil {
		return irconfig.Document{}, err
	}
	token, err := s.DelegatedToken(ctx, session, raw, "platform", []string{coreauth.ScopeSecretsRead, coreauth.ScopeSecretsReveal}, project)
	if err != nil {
		return irconfig.Document{}, err
	}
	client, err := common.NewConfigurationClient(s.cfg.Platform.APIBaseURL)
	if err != nil {
		return irconfig.Document{}, ErrUnavailable
	}
	release, err := s.lockGatewayConfiguration(ctx, project)
	if err != nil {
		return irconfig.Document{}, err
	}
	defer release()
	document, err := client.Read(ctx, token, project)
	if err != nil {
		return document, configurationError(err)
	}
	return document, s.syncGatewayConfiguration(ctx, project, document)
}

func (s *Service) PutConfiguration(ctx context.Context, raw, project string, values []byte) (irconfig.Metadata, error) {
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return irconfig.Metadata{}, err
	}
	if err = s.CheckProject(ctx, session, raw, project); err != nil {
		return irconfig.Metadata{}, err
	}
	token, err := s.DelegatedToken(ctx, session, raw, "platform", []string{coreauth.ScopeSecretsRead, coreauth.ScopeSecretsCreate, coreauth.ScopeSecretsWrite}, project)
	if err != nil {
		return irconfig.Metadata{}, err
	}
	client, err := common.NewConfigurationClient(s.cfg.Platform.APIBaseURL)
	if err != nil {
		return irconfig.Metadata{}, ErrUnavailable
	}
	release, err := s.lockGatewayConfiguration(ctx, project)
	if err != nil {
		return irconfig.Metadata{}, err
	}
	defer release()
	metadata, err := client.Put(ctx, token, project, values)
	if err != nil {
		return metadata, configurationError(err)
	}
	document := irconfig.Document{Metadata: metadata, Value: string(values)}
	return metadata, s.syncGatewayConfiguration(ctx, project, document)
}

func (s *Service) GatewayConfigToken(ctx context.Context, raw, project string) (string, error) {
	if IsAgentBearer(raw) {
		_, err := s.VerifyAgent(ctx, raw, project)
		return "", err
	}
	session, _, err := s.Authenticate(ctx, raw)
	if err != nil {
		return "", err
	}
	return s.DelegatedToken(ctx, session, raw, "platform", []string{coreauth.ScopeSecretsRead, coreauth.ScopeSecretsReveal}, project)
}
