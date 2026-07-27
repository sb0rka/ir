package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/output"
)

// CreateReport Сгенерировать отчёт по инциденту
// (POST /investigations/{investigation_id}/reports)
func (s *Server) CreateReport(ctx context.Context, request output.CreateReportRequestObject) (output.CreateReportResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// CreateResponsePackage Собрать пакет реагирования
// (POST /investigations/{investigation_id}/response-packages)
func (s *Server) CreateResponsePackage(ctx context.Context, request output.CreateResponsePackageRequestObject) (output.CreateResponsePackageResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetReport Статус отчёта
// (GET /reports/{report_id})
func (s *Server) GetReport(ctx context.Context, request output.GetReportRequestObject) (output.GetReportResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// DownloadReport Скачать отчёт
// (GET /reports/{report_id}/content)
func (s *Server) DownloadReport(ctx context.Context, request output.DownloadReportRequestObject) (output.DownloadReportResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// GetResponsePackage Пакет реагирования
// (GET /response-packages/{package_id})
func (s *Server) GetResponsePackage(ctx context.Context, request output.GetResponsePackageRequestObject) (output.GetResponsePackageResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

// FinalizeResponsePackage Зафиксировать пакет
// (PATCH /response-packages/{package_id})
func (s *Server) FinalizeResponsePackage(ctx context.Context, request output.FinalizeResponsePackageRequestObject) (output.FinalizeResponsePackageResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
