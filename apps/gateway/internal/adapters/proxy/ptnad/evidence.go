package ptnad

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type exportTask struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Result struct {
		URL       string            `json:"url"`
		Errors    []json.RawMessage `json:"errors"`
		Total     int               `json:"total_files"`
		Extracted int               `json:"extracted_files"`
	} `json:"result"`
}

func (provider *Provider) StartEvidence(ctx context.Context, access capability.Access, ref domain.EvidenceReference) (capability.EvidenceHandle, error) {
	handle := capability.EvidenceHandle{Reference: ref, State: "pending"}
	kind := SessionRecordType
	if ref.Kind == "payload" {
		kind = AttackRecordType
	}
	store, window, err := provider.validateObjectRef(ref.Ref, kind)
	if err != nil {
		return handle, err
	}
	cookie := Access{Cookie: access.Cookie}
	switch ref.Kind {
	case "payload":
		if ref.ObjectID != "" {
			return handle, invalidRequest("payload uses its alert ref without object_id")
		}
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
	case "file":
		if ref.ObjectID == "" {
			return handle, invalidRequest("file object_id is required")
		}
		session, err := provider.client.GetSession(ctx, SessionRef{StoreID: store, ExternalID: ref.Ref.ExternalID, TimeRange: window}, cookie)
		if err != nil {
			return handle, canonicalProviderError(err)
		}
		hash := ""
		for _, file := range session.Files {
			if file.ExternalID == ref.ObjectID {
				hash = file.MD5
				break
			}
		}
		if hash == "" {
			return handle, fmt.Errorf("%w: file is absent or has no extraction hash", domain.ErrNotFound)
		}
		body, _ := json.Marshal(struct {
			IDs     []string `json:"id"`
			Start   int64    `json:"start"`
			End     int64    `json:"end"`
			Sources []string `json:"source"`
			MD5     []string `json:"md5"`
		}{[]string{ref.Ref.ExternalID}, window.From.UnixMilli(), window.To.UnixMilli(), []string{strconv.FormatInt(store, 10)}, []string{hash}})
		var task exportTask
		if err := provider.client.doJSON(ctx, "evidence export", http.MethodPost, "api/v2/sources/getfile", "", string(body), cookie, &task); err != nil {
			return handle, canonicalProviderError(err)
		}
		if !validTaskID(task.ID) {
			return handle, invalidRequest("NAD returned an invalid export task")
		}
		handle.TaskID = task.ID
		handle.Filename = "evidence.zip"
		handle.ContentType = "application/zip"
		return updateExportTask(handle, task)
	case "pcap":
		// Do not guess an export route from a metadata ID. Capturing this vendor
		// contract on the lab is a release prerequisite for PCAP support.
		return handle, fmt.Errorf("%w: NAD PCAP export contract is not verified", domain.ErrUnsupportedCapability)
	default:
		return handle, invalidRequest("unsupported evidence kind")
	}
}

func validTaskID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func updateExportTask(handle capability.EvidenceHandle, task exportTask) (capability.EvidenceHandle, error) {
	if task.ID != handle.TaskID {
		return handle, &ProtocolError{Operation: "evidence task identity"}
	}
	switch strings.ToUpper(task.State) {
	case "PENDING", "STARTED", "PROGRESS", "RETRY":
		handle.State = "pending"
	case "SUCCESS":
		if task.Result.URL != "/api/v2/download/"+handle.TaskID+".zip" {
			return handle, &ProtocolError{Operation: "evidence download location"}
		}
		if task.Result.Total < 0 || task.Result.Extracted < 0 || task.Result.Extracted > task.Result.Total {
			return handle, &ProtocolError{Operation: "evidence task counts"}
		}
		handle.State = "ready"
		if len(task.Result.Errors) > 0 || task.Result.Extracted != task.Result.Total {
			handle.State = "partial"
			handle.Error = "NAD could not extract every selected file"
		}
		if task.Result.Extracted == 0 {
			handle.State = "failed"
			handle.Error = "NAD extracted no files"
		}
	case "FAILURE", "REVOKED":
		handle.State = "failed"
		handle.Error = "NAD evidence export failed"
	default:
		return handle, &ProtocolError{Operation: "evidence task state"}
	}
	return handle, nil
}

func (provider *Provider) PollEvidence(ctx context.Context, access capability.Access, handle capability.EvidenceHandle) (capability.EvidenceHandle, error) {
	if handle.State != "pending" {
		return handle, nil
	}
	if !validTaskID(handle.TaskID) {
		return handle, invalidRequest("invalid export task")
	}
	var task exportTask
	if err := provider.client.doJSON(ctx, "evidence task", http.MethodGet, "api/v2/tasks/"+handle.TaskID, "", "", Access{Cookie: access.Cookie}, &task); err != nil {
		return handle, canonicalProviderError(err)
	}
	result, err := updateExportTask(handle, task)
	return result, canonicalProviderError(err)
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
	if handle.Reference.Kind == "payload" {
		data, err := provider.payload(ctx, Access{Cookie: access.Cookie}, handle)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	if handle.Reference.Kind != "file" || !validTaskID(handle.TaskID) {
		return nil, invalidRequest("invalid export handle")
	}
	cookie, err := validateAccess(Access{Cookie: access.Cookie})
	if err != nil {
		return nil, err
	}
	endpoint := provider.client.baseURL.ResolveReference(&url.URL{Path: "api/v2/download/" + handle.TaskID + ".zip"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Cookie", cookie)
	response, err := provider.client.downloadHTTP.Do(request)
	if err != nil {
		return nil, canonicalProviderError(&TransportError{Operation: "evidence download"})
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, canonicalProviderError(&ResponseError{Operation: "evidence download", StatusCode: response.StatusCode})
	}
	return response.Body, nil
}
