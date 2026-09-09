package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const evidenceTTL = time.Hour
const maxEvidenceExports = 1024

type evidenceEntry struct {
	mu      sync.Mutex
	project string
	expires time.Time
	handle  capability.EvidenceHandle
}

type EvidenceExport struct {
	ID        string
	ExpiresAt time.Time
	Handle    capability.EvidenceHandle
}

func (service *Service) StartEvidence(ctx context.Context, access ProjectAccess, ref domain.EvidenceReference) (EvidenceExport, error) {
	provider, ok := service.registry.Provider(ref.Ref.SourceCode)
	if !ok || provider.Evidence == nil {
		return EvidenceExport{}, domain.ErrUnsupportedCapability
	}
	if access.ProjectID == "" {
		return EvidenceExport{}, domain.ErrNotFound
	}
	// Reserve capacity before starting a vendor task. No automatic POST retry:
	// a lost response may otherwise start a second independent extraction.
	id := uuid.NewString()
	now := time.Now()
	entry := &evidenceEntry{project: access.ProjectID, expires: now.Add(evidenceTTL)}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	service.exportsMu.Lock()
	if service.exports == nil {
		service.exports = make(map[string]*evidenceEntry)
	}
	for key, value := range service.exports {
		if now.After(value.expires.Add(evidenceTTL)) {
			delete(service.exports, key)
		}
	}
	if len(service.exports) >= maxEvidenceExports {
		// Retain expired metadata when possible, but never let it block a new export.
		for key, value := range service.exports {
			if now.After(value.expires) {
				delete(service.exports, key)
				if len(service.exports) < maxEvidenceExports {
					break
				}
			}
		}
	}
	if len(service.exports) >= maxEvidenceExports {
		service.exportsMu.Unlock()
		return EvidenceExport{}, &domain.RequestError{Code: "source_unavailable", Message: "evidence export capacity reached"}
	}
	service.exports[id] = entry
	service.exportsMu.Unlock()
	credentials, err := service.loadCredentials(ctx, access, provider, false)
	if err == nil {
		taskCtx, cancel := context.WithTimeout(ctx, service.sourceTimeout)
		entry.handle, err = provider.Evidence.StartEvidence(taskCtx, capability.Access{Cookie: credentials.cookie}, ref)
		cancel()
	}
	if err != nil {
		service.exportsMu.Lock()
		delete(service.exports, id)
		service.exportsMu.Unlock()
		if errors.Is(err, domain.ErrUnsupportedCapability) {
			return EvidenceExport{}, err
		}
		return EvidenceExport{}, providerError(err)
	}
	return EvidenceExport{ID: id, ExpiresAt: entry.expires, Handle: entry.handle}, nil
}

func (service *Service) evidenceEntry(access ProjectAccess, id string) (*evidenceEntry, error) {
	service.exportsMu.Lock()
	entry := service.exports[id]
	service.exportsMu.Unlock()
	if entry == nil || entry.project != access.ProjectID {
		return nil, domain.ErrNotFound
	}
	return entry, nil
}

// EvidenceSourceCode does not poll NAD. Transport uses it to recheck the current
// allowlist on status and content requests, including already-ready exports.
func (service *Service) EvidenceSourceCode(access ProjectAccess, id string) (string, error) {
	entry, err := service.evidenceEntry(access, id)
	if err != nil {
		return "", err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.handle.Reference.Ref.SourceCode, nil
}

func (service *Service) GetEvidence(ctx context.Context, access ProjectAccess, id string) (EvidenceExport, error) {
	entry, err := service.evidenceEntry(access, id)
	if err != nil {
		return EvidenceExport{}, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if time.Now().After(entry.expires) {
		handle := entry.handle
		handle.State = "expired"
		return EvidenceExport{ID: id, ExpiresAt: entry.expires, Handle: handle}, nil
	}
	provider, ok := service.registry.Provider(entry.handle.Reference.Ref.SourceCode)
	if !ok || provider.Evidence == nil {
		return EvidenceExport{}, domain.ErrNotFound
	}
	if entry.handle.State == "pending" {
		err := service.callProvider(ctx, access, provider, func(ctx context.Context, a capability.Access) error {
			result, err := provider.Evidence.PollEvidence(ctx, a, entry.handle)
			if err == nil {
				entry.handle = result
			}
			return err
		})
		if err != nil {
			return EvidenceExport{}, err
		}
	}
	return EvidenceExport{ID: id, ExpiresAt: entry.expires, Handle: entry.handle}, nil
}

func (service *Service) ReadEvidence(ctx context.Context, access ProjectAccess, id string, consume func(EvidenceExport, io.Reader) error) error {
	export, err := service.GetEvidence(ctx, access, id)
	if err != nil {
		return err
	}
	if export.Handle.State == "expired" {
		return &domain.RequestError{Code: "evidence_expired", Message: "evidence export expired"}
	}
	if export.Handle.State != "ready" && export.Handle.State != "partial" {
		return &domain.RequestError{Code: "evidence_not_ready", Message: "evidence export is not ready"}
	}
	provider, ok := service.registry.Provider(export.Handle.Reference.Ref.SourceCode)
	if !ok || provider.Evidence == nil {
		return domain.ErrNotFound
	}
	credentials, err := service.loadCredentials(ctx, access, provider, false)
	if err != nil {
		return err
	}
	// Keep request cancellation alive until the downstream finishes consuming.
	// Once bytes have been sent a retry would corrupt the stream.
	downloadCtx, cancel := context.WithDeadline(ctx, export.ExpiresAt)
	defer cancel()
	content, err := provider.Evidence.OpenEvidence(downloadCtx, capability.Access{Cookie: credentials.cookie}, export.Handle)
	if err != nil {
		return providerError(err)
	}
	defer content.Close()
	if err := consume(export, content); err != nil {
		return fmt.Errorf("evidence content: %w", err)
	}
	return nil
}
