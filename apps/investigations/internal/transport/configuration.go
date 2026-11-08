package transport

import (
	"io"
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	"github.com/sb0rka/ir/packages/common/irconfig"
	"github.com/sb0rka/ir/packages/contract/authentication"
)

func (h *AuthHandler) configurationProject(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	_, raw, ok := h.authenticated(w, r)
	if !ok {
		return "", "", false
	}
	project := r.Header.Get(projectIDHeader)
	if !validProjectID(project) {
		writeAuthError(w, h.log, authflow.ErrInvalid)
		return "", "", false
	}
	return raw, project, true
}

func (h *AuthHandler) GetIRConfiguration(w http.ResponseWriter, r *http.Request, _ authentication.GetIRConfigurationParams) {
	raw, project, ok := h.configurationProject(w, r)
	if !ok {
		return
	}
	document, err := h.flow.Configuration(r.Context(), raw, project)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, document)
}

func (h *AuthHandler) PutIRConfiguration(w http.ResponseWriter, r *http.Request, _ authentication.PutIRConfigurationParams) {
	if !h.origin(w, r) {
		return
	}
	raw, project, ok := h.configurationProject(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, irconfig.MaxRequestBytes)
	values, err := io.ReadAll(r.Body)
	if err != nil {
		writeAuthError(w, h.log, authflow.ErrInvalid)
		return
	}
	metadata, err := h.flow.PutConfiguration(r.Context(), raw, project, values)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, metadata)
}
