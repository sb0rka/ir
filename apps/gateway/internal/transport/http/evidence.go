package httptransport

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	coretransport "github.com/sb0rka/sb0rka/packages/core/transport"
)

func evidenceRefToAPI(ref domain.EvidenceReference) api.EvidenceReference {
	return api.EvidenceReference{Kind: api.EvidenceReferenceKind(ref.Kind), Ref: sourceObjectRefToAPI(ref.Ref), ObjectId: stringPointer(ref.ObjectID)}
}
func evidenceToAPI(refs []domain.EvidenceReference) *[]api.EvidenceReference {
	if len(refs) == 0 {
		return nil
	}
	items := make([]api.EvidenceReference, 0, len(refs))
	for _, ref := range refs {
		items = append(items, evidenceRefToAPI(ref))
	}
	return &items
}
func evidenceExportToAPI(value service.EvidenceExport) api.EvidenceExport {
	return api.EvidenceExport{ExportId: uuid.MustParse(value.ID), Evidence: evidenceRefToAPI(value.Handle.Reference), State: api.EvidenceExportState(value.Handle.State), ExpiresAt: value.ExpiresAt,
		Filename: stringPointer(value.Handle.Filename), ContentType: stringPointer(value.Handle.ContentType), Size: value.Handle.Size, Error: stringPointer(value.Handle.Error)}
}

func (server *Server) CreateEvidenceExport(w http.ResponseWriter, r *http.Request, _ api.CreateEvidenceExportParams) {
	var body api.EvidenceReference
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, 400, "bad_request", err.Error())
		return
	}
	ref, err := server.sourceObjectRefFromAPI(body.Ref)
	if err != nil {
		respondError(w, 400, "bad_request", err.Error())
		return
	}
	if !server.sourceAllowed(r.Context(), ref.SourceCode) {
		server.writeServiceError(w, domain.ErrNotFound)
		return
	}
	if !server.sourceInstanceAllowed(ref.SourceCode, ref.SourceInstance) {
		respondError(w, 400, "bad_request", "source_instance is not configured")
		return
	}
	result, err := server.service.StartEvidence(r.Context(), projectAccess(r), domain.EvidenceReference{Kind: string(body.Kind), Ref: ref, ObjectID: stringValue(body.ObjectId)})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusAccepted, evidenceExportToAPI(result))
}

func (server *Server) evidenceAllowed(r *http.Request, id string) error {
	source, err := server.service.EvidenceSourceCode(projectAccess(r), id)
	if err != nil || !server.sourceAllowed(r.Context(), source) {
		return domain.ErrNotFound
	}
	return nil
}

func (server *Server) GetEvidenceExport(w http.ResponseWriter, r *http.Request, id api.ExportId, _ api.GetEvidenceExportParams) {
	if err := server.evidenceAllowed(r, id.String()); err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.GetEvidence(r.Context(), projectAccess(r), id.String())
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusOK, evidenceExportToAPI(result))
}

func (server *Server) GetEvidenceContent(w http.ResponseWriter, r *http.Request, id api.ExportId, params api.GetEvidenceContentParams) {
	if err := server.evidenceAllowed(r, id.String()); err != nil {
		server.writeServiceError(w, err)
		return
	}
	var offset int64
	if params.Offset != nil {
		offset = *params.Offset
	}
	if offset < 0 || params.Limit != nil && (*params.Limit < 1 || *params.Limit > 65536) {
		respondError(w, 400, "bad_request", "invalid byte slice")
		return
	}
	started := false
	err := server.service.ReadEvidence(r.Context(), projectAccess(r), id.String(), func(export service.EvidenceExport, reader io.Reader) error {
		// Replace the server's ordinary API write timeout only for this download.
		writer := w
		// core v0.0.2 Recorder has no Unwrap; keep its identity for panic recovery.
		if recorder, ok := w.(*coretransport.Recorder); ok {
			writer = recorder.ResponseWriter
		}
		if err := http.NewResponseController(writer).SetWriteDeadline(export.ExpiresAt); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if offset > 0 {
			if _, err := io.CopyN(io.Discard, reader, offset); err != nil {
				if errors.Is(err, io.EOF) {
					return &domain.RequestError{Code: "evidence_range", Message: "offset exceeds evidence size"}
				}
				return err
			}
		}
		var chunk []byte
		if params.Limit != nil {
			var err error
			chunk, err = io.ReadAll(io.LimitReader(reader, int64(*params.Limit)+1))
			if err != nil {
				return err
			}
			eof := len(chunk) <= *params.Limit
			if !eof {
				chunk = chunk[:*params.Limit]
			}
			w.Header().Set("X-Evidence-EOF", strconv.FormatBool(eof))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
		}
		w.Header().Set("Content-Type", export.Handle.ContentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": export.Handle.Filename}))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		started = true
		w.WriteHeader(http.StatusOK)
		if params.Limit != nil {
			_, err := w.Write(chunk)
			return err
		}
		_, err := io.Copy(w, reader)
		return err
	})
	if err == nil {
		return
	}
	if started {
		panic(http.ErrAbortHandler)
	}
	var requestErr *domain.RequestError
	if errors.As(err, &requestErr) {
		status := 0
		switch requestErr.Code {
		case "evidence_expired":
			status = 410
		case "evidence_not_ready":
			status = 409
		case "evidence_range":
			status = 416
		}
		if status != 0 {
			respondError(w, status, requestErr.Code, requestErr.Message)
			return
		}
	}
	server.writeServiceError(w, err)
}
