package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (service *Service) GetAccountUserinfo(ctx context.Context, access ProjectAccess, source string) (domain.AccountUserinfo, error) {
	providers, err := service.registry.Select([]string{source}, domain.CapabilityAccountUserinfo)
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	provider := providers[0]
	requestCtx, cancel := context.WithTimeout(ctx, service.requestTimeout)
	defer cancel()
	var userinfo domain.AccountUserinfo
	err = service.callProviderWithCredentialReload(requestCtx, access, provider, true, func(attemptCtx context.Context, providerAccess capability.Access) error {
		var innerErr error
		userinfo, innerErr = provider.AccountUserinfo.GetAccountUserinfo(attemptCtx, providerAccess)
		return innerErr
	})
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	if userinfo.SourceCode != provider.Source.Code || strings.TrimSpace(userinfo.UserName) == "" {
		return domain.AccountUserinfo{}, fmt.Errorf("source returned invalid account userinfo")
	}
	return userinfo, nil
}
