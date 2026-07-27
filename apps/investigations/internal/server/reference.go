package server

import (
	"context"

	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/reference"
)

// GetReference Every dictionary the UI needs
// (GET /reference)
func (s *Server) GetReference(ctx context.Context, request reference.GetReferenceRequestObject) (reference.GetReferenceResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
