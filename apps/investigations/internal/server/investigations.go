package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

func (s *Server) ListInvestigations(
	_ context.Context, _ investigations.ListInvestigationsRequestObject,
) (investigations.ListInvestigationsResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) CreateInvestigation(
	_ context.Context, _ investigations.CreateInvestigationRequestObject,
) (investigations.CreateInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetInvestigation(
	_ context.Context, _ investigations.GetInvestigationRequestObject,
) (investigations.GetInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) UpdateInvestigation(
	_ context.Context, _ investigations.UpdateInvestigationRequestObject,
) (investigations.UpdateInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetInvestigationTree(
	_ context.Context, _ investigations.GetInvestigationTreeRequestObject,
) (investigations.GetInvestigationTreeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteInvestigation(ctx context.Context, request investigations.DeleteInvestigationRequestObject) (investigations.DeleteInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
