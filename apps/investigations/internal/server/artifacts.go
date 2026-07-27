package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/artifacts"
)

// Обогатить артефакт расширением
// POST /artifacts/{artifact_id}/enrich
func (s *Server) EnrichArtifact(
	_ context.Context, _ artifacts.EnrichArtifactRequestObject,
) (artifacts.EnrichArtifactResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Артефакты расследования
// GET /investigations/{investigation_id}/artifacts
func (s *Server) ListArtifacts(
	_ context.Context, _ artifacts.ListArtifactsRequestObject,
) (artifacts.ListArtifactsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// Массовый ввод индикаторов
// POST /investigations/{investigation_id}/iocs
func (s *Server) ImportIocs(
	_ context.Context, _ artifacts.ImportIocsRequestObject,
) (artifacts.ImportIocsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
