package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type evidenceFake struct {
	starts, polls, opens int
	startErr             error
	state                string
	ctx                  context.Context
	closed               bool
}

func (f *evidenceFake) StartEvidence(_ context.Context, _ capability.Access, ref domain.EvidenceReference) (capability.EvidenceHandle, error) {
	f.starts++
	return capability.EvidenceHandle{Reference: ref, State: f.state, ParentID: "private-vendor-id"}, f.startErr
}
func (f *evidenceFake) PollEvidence(_ context.Context, _ capability.Access, h capability.EvidenceHandle) (capability.EvidenceHandle, error) {
	f.polls++
	h.State = f.state
	return h, nil
}
func (f *evidenceFake) OpenEvidence(ctx context.Context, _ capability.Access, _ capability.EvidenceHandle) (io.ReadCloser, error) {
	f.opens++
	f.ctx = ctx
	return &evidenceReader{Reader: strings.NewReader("exact content"), closed: &f.closed}, nil
}

type evidenceReader struct {
	io.Reader
	closed *bool
}

func (r *evidenceReader) Close() error { *r.closed = true; return nil }
func evidenceService(t *testing.T, f *evidenceFake) *Service {
	t.Helper()
	reg, err := registry.New(registry.Provider{Source: domain.Source{Code: "nad"}, CredentialSecret: "cookie", Evidence: f})
	if err != nil {
		t.Fatal(err)
	}
	return New(reg, &aggregationSecrets{}, time.Second, time.Second)
}
func TestEvidenceProjectTTLAndStreamingLifetime(t *testing.T) {
	f := &evidenceFake{state: "pending"}
	s := evidenceService(t, f)
	access := ProjectAccess{ProjectID: "aabbccddee", Bearer: "test"}
	exp, err := s.StartEvidence(context.Background(), access, domain.EvidenceReference{Kind: "file", Ref: domain.SourceObjectRef{SourceCode: "nad"}})
	if err != nil {
		t.Fatal(err)
	}
	if exp.ExpiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Fatal("TTL")
	}
	foreign := ProjectAccess{ProjectID: "1122334455"}
	if _, err = s.GetEvidence(context.Background(), foreign, exp.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if err = s.ReadEvidence(context.Background(), foreign, exp.ID, func(EvidenceExport, io.Reader) error { t.Fatal("foreign content"); return nil }); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if f.polls != 0 || f.opens != 0 {
		t.Fatal("foreign request reached provider")
	}
	if err = s.ReadEvidence(context.Background(), access, exp.ID, func(EvidenceExport, io.Reader) error { t.Fatal("pending content"); return nil }); err == nil {
		t.Fatal("pending accepted")
	}
	f.state = "partial"
	ctx, cancel := context.WithCancel(context.Background())
	err = s.ReadEvidence(ctx, access, exp.ID, func(e EvidenceExport, r io.Reader) error {
		if f.ctx.Err() != nil || e.Handle.State != "partial" {
			t.Fatal("context closed before streaming")
		}
		raw, _ := io.ReadAll(r)
		if string(raw) != "exact content" {
			t.Fatal("bytes changed")
		}
		cancel()
		if !errors.Is(f.ctx.Err(), context.Canceled) {
			t.Fatal("cancellation not propagated")
		}
		return io.ErrUnexpectedEOF
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) || !f.closed || f.opens != 1 {
		t.Fatal(err, f)
	}
	entry, _ := s.evidenceEntry(access, exp.ID)
	entry.expires = time.Now().Add(-time.Second)
	exp, err = s.GetEvidence(context.Background(), access, exp.ID)
	if err != nil || exp.Handle.State != "expired" {
		t.Fatal(exp, err)
	}
	err = s.ReadEvidence(context.Background(), access, exp.ID, func(EvidenceExport, io.Reader) error { t.Fatal("expired content"); return nil })
	var reqErr *domain.RequestError
	if !errors.As(err, &reqErr) || reqErr.Code != "evidence_expired" {
		t.Fatal(err)
	}
	restarted := evidenceService(t, f)
	if _, err = restarted.GetEvidence(context.Background(), access, exp.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("export survived restart")
	}
}
func TestEvidencePostNotRetriedAndErrorSanitized(t *testing.T) {
	f := &evidenceFake{state: "pending", startErr: &domain.UpstreamError{StatusCode: 503}}
	s := evidenceService(t, f)
	_, err := s.StartEvidence(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "test"}, domain.EvidenceReference{Ref: domain.SourceObjectRef{SourceCode: "nad"}})
	if err == nil || f.starts != 1 || len(s.exports) != 0 {
		t.Fatal(err, f.starts, len(s.exports))
	}
	f.startErr = domain.ErrUnsupportedCapability
	_, err = s.StartEvidence(context.Background(), ProjectAccess{ProjectID: "aabbccddee", Bearer: "test"}, domain.EvidenceReference{Ref: domain.SourceObjectRef{SourceCode: "nad"}})
	if !errors.Is(err, domain.ErrUnsupportedCapability) {
		t.Fatal(err)
	}
}
func TestEvidenceCapacityReclaimsExpiredEntries(t *testing.T) {
	f := &evidenceFake{state: "ready"}
	s := evidenceService(t, f)
	access := ProjectAccess{ProjectID: "aabbccddee", Bearer: "test"}
	ref := domain.EvidenceReference{Ref: domain.SourceObjectRef{SourceCode: "nad"}}
	for i := 0; i < maxEvidenceExports; i++ {
		if _, err := s.StartEvidence(context.Background(), access, ref); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartEvidence(context.Background(), access, ref); err == nil {
		t.Fatal("active capacity exceeded")
	}
	for _, entry := range s.exports {
		entry.expires = time.Now().Add(-time.Minute)
	}
	if _, err := s.StartEvidence(context.Background(), access, ref); err != nil {
		t.Fatal("expired entries blocked export:", err)
	}
	if len(s.exports) > maxEvidenceExports {
		t.Fatal("registry exceeded bound")
	}
}

func TestSearchCursorBindsNewControls(t *testing.T) {
	window := domain.TimeRange{From: time.Unix(1, 0), To: time.Unix(2, 0)}
	legacy := objectFingerprint([]string{"nad"}, []string{"nad_session"}, window)
	if legacy != objectFingerprint([]string{"nad"}, []string{"nad_session"}, window, "") || legacy != objectFingerprint([]string{"nad"}, []string{"nad_session"}, window, "", "") {
		t.Fatal("empty controls broke old cursors")
	}
	original := objectFingerprint([]string{"nad"}, []string{"nad_session"}, window, "host.port == 2222")
	cursor, err := encodeCursor(cursorState{Fingerprint: original, Positions: map[string]string{"nad": "next"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{legacy, objectFingerprint([]string{"nad"}, []string{"nad_session"}, window, "host.port == 3333"), objectFingerprint([]string{"nad"}, []string{"nad_session"}, window, "host.port == 2222", createdRangeKey(&window))} {
		if _, err := decodeCursor(cursor, next); err == nil {
			t.Fatal("cursor reused with changed conditions")
		}
	}
}
