package ptnad

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (provider *Provider) StartEvidence(ctx context.Context, access capability.Access, ref domain.EvidenceReference) (capability.EvidenceHandle, error) {
	handle := capability.EvidenceHandle{Reference: ref, State: "pending"}
	if ref.Kind != "payload" {
		return handle, fmt.Errorf("%w: NAD exports alert payload only; file extraction is disabled and PCAP is supplied with the case", domain.ErrUnsupportedCapability)
	}
	store, window, err := provider.validateObjectRef(ref.Ref, AttackRecordType)
	if err != nil {
		return handle, err
	}
	if ref.ObjectID != "" {
		return handle, invalidRequest("payload uses its alert ref without object_id")
	}
	cookie := Access{Cookie: access.Cookie}
	attack, err := provider.client.getAttack(ctx, AttackRef{StoreID: store, ExternalID: ref.Ref.ExternalID, TimeRange: window}, cookie, false)
	if err != nil {
		return handle, canonicalProviderError(err)
	}
	if attack.ParentSession == nil {
		return handle, fmt.Errorf("%w: alert parent", domain.ErrNotFound)
	}
	handle.ParentID = attack.ParentSession.ExternalID
	data, err := provider.payload(ctx, cookie, handle)
	if err != nil {
		return handle, err
	}
	size := int64(len(data))
	handle.Size = &size
	handle.State = "ready"
	handle.Filename = "payload.bin"
	handle.ContentType = "application/octet-stream"
	return handle, nil
}

func (provider *Provider) PollEvidence(_ context.Context, _ capability.Access, handle capability.EvidenceHandle) (capability.EvidenceHandle, error) {
	if handle.Reference.Kind != "payload" {
		return handle, domain.ErrUnsupportedCapability
	}
	// NAD payload is ready at creation; there is no vendor extraction task.
	return handle, nil
}

func (provider *Provider) payload(ctx context.Context, access Access, handle capability.EvidenceHandle) ([]byte, error) {
	ref := handle.Reference.Ref
	store, window, err := provider.validateObjectRef(ref, AttackRecordType)
	if err != nil {
		return nil, err
	}
	detail, err := provider.client.attackDetail(ctx, AttackRef{StoreID: store, ExternalID: ref.ExternalID, TimeRange: window}, handle.ParentID, access)
	if err != nil {
		return nil, canonicalProviderError(err)
	}
	if detail.Payload == "" {
		return nil, fmt.Errorf("%w: alert payload", domain.ErrNotFound)
	}
	data, err := base64.StdEncoding.DecodeString(detail.Payload)
	if err != nil {
		return nil, canonicalProviderError(&ProtocolError{Operation: "payload encoding"})
	}
	return data, nil
}

func (provider *Provider) OpenEvidence(ctx context.Context, access capability.Access, handle capability.EvidenceHandle) (io.ReadCloser, error) {
	if handle.Reference.Kind != "payload" {
		return nil, domain.ErrUnsupportedCapability
	}
	data, err := provider.payload(ctx, Access{Cookie: access.Cookie}, handle)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
