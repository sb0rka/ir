package service

import "github.com/sb0rka/ir/apps/gateway/internal/domain"

func (service *Service) ListSources() []domain.Source {
	return service.registry.Sources()
}
