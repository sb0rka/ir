package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/admin"
)

// Журнал действий
// GET /audit
func (s *Server) ListAudit(
	_ context.Context, _ admin.ListAuditRequestObject,
) (admin.ListAuditResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Расширения обогащения
// GET /extensions
func (s *Server) ListExtensions(
	_ context.Context, _ admin.ListExtensionsRequestObject,
) (admin.ListExtensionsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Состояние источников и расширений
// GET /health
func (s *Server) GetHealth(
	_ context.Context, _ admin.GetHealthRequestObject,
) (admin.GetHealthResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Метрики расследования
// GET /investigations/{investigation_id}/metrics
func (s *Server) GetInvestigationMetrics(
	_ context.Context, _ admin.GetInvestigationMetricsRequestObject,
) (admin.GetInvestigationMetricsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Правила связывания
// GET /linking-rules
func (s *Server) ListLinkingRules(
	_ context.Context, _ admin.ListLinkingRulesRequestObject,
) (admin.ListLinkingRulesResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Источники улик
// GET /sources
func (s *Server) ListSources(
	ctx context.Context, _ admin.ListSourcesRequestObject,
) (admin.ListSourcesResponseObject, error) {
	items, err := s.db.ListSources(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]admin.Source, 0, len(items))
	for _, item := range items {
		out = append(out, admin.Source{
			Code:      item.Code,
			Kind:      admin.SourceKind(item.Kind),
			Title:     item.Title,
			SecretRef: item.SecretRef,
			IsEnabled: item.IsEnabled,
		})
	}
	return admin.ListSources200JSONResponse{Items: out}, nil
}
