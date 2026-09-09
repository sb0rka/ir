package capability

import (
	"context"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"io"
)

// EvidenceHandle is private adapter state, never serialized to a client.
type EvidenceHandle struct {
	Reference   domain.EvidenceReference
	ParentID    string
	State       string
	Filename    string
	ContentType string
	Size        *int64
	Error       string
}

type EvidenceSource interface {
	StartEvidence(context.Context, Access, domain.EvidenceReference) (EvidenceHandle, error)
	PollEvidence(context.Context, Access, EvidenceHandle) (EvidenceHandle, error)
	OpenEvidence(context.Context, Access, EvidenceHandle) (io.ReadCloser, error)
}
