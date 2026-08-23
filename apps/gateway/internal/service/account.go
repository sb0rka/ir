package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

func (service *Service) GetAccountUserinfo(ctx context.Context, access ProjectAccess, source string) (domain.AccountUserinfo, error) {
	providers, err := service.registry.Select([]string{source}, domain.CapabilityAccountUserinfo)
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	return service.accountUserinfo(requestCtx, access, providers[0], true)
}

func (service *Service) accountUserinfo(ctx context.Context, access ProjectAccess, provider registry.Provider, forceReload bool) (domain.AccountUserinfo, error) {
	credentials, err := service.loadCredentials(ctx, access, provider, forceReload)
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	userinfo, err := service.callAccountUserinfo(ctx, provider, credentials)
	if err == nil {
		return userinfo, nil
	}
	if !retryableAccountError(ctx, err) {
		return domain.AccountUserinfo{}, accountError(err)
	}
	credentials, reloadErr := service.loadCredentials(ctx, access, provider, true)
	if reloadErr != nil {
		return domain.AccountUserinfo{}, reloadErr
	}
	userinfo, err = service.callAccountUserinfo(ctx, provider, credentials)
	if err != nil {
		return domain.AccountUserinfo{}, accountError(err)
	}
	return userinfo, nil
}

func (service *Service) callAccountUserinfo(ctx context.Context, provider registry.Provider, credentials credentialSnapshot) (domain.AccountUserinfo, error) {
	sourceCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
	defer cancel()
	userinfo, err := provider.AccountUserinfo.GetAccountUserinfo(sourceCtx, capability.AccountUserinfoRequest{
		BaseURL:     credentials.baseURL,
		Credential:  credentials.credential,
		Timeout:     service.sourceTimeout,
		SkipTLSVerify: service.skipTLSVerify,
	})
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	if userinfo.SourceCode != provider.Source.Code || strings.TrimSpace(userinfo.UserName) == "" {
		return domain.AccountUserinfo{}, fmt.Errorf("source returned invalid account userinfo")
	}
	return userinfo, nil
}
