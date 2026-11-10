package transport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/authflow"
	"github.com/sb0rka/ir/packages/contract/authentication"
)

func (h *AuthHandler) agentAccessProject(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	if len(r.Header.Values("Authorization")) != 1 || len(r.Header.Values(projectIDHeader)) != 1 {
		writeAuthError(w, h.log, authflow.ErrUnauthorized)
		return "", "", false
	}
	return h.configurationProject(w, r)
}

func (h *AuthHandler) GetIRAgentAccess(w http.ResponseWriter, r *http.Request, _ authentication.GetIRAgentAccessParams) {
	raw, project, ok := h.agentAccessProject(w, r)
	if !ok {
		return
	}
	metadata, err := h.flow.AgentAccess(r.Context(), raw, project)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, metadata)
}

func (h *AuthHandler) CreateIRAgentAccess(w http.ResponseWriter, r *http.Request, _ authentication.CreateIRAgentAccessParams) {
	if !h.origin(w, r) {
		return
	}
	raw, project, ok := h.agentAccessProject(w, r)
	if !ok {
		return
	}
	issued, err := h.flow.IssueAgentAccess(r.Context(), raw, project, 0)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	authJSON(w, issued)
}

func (h *AuthHandler) RotateIRAgentAccess(w http.ResponseWriter, r *http.Request, _ authentication.RotateIRAgentAccessParams) {
	if !h.origin(w, r) {
		return
	}
	raw, project, ok := h.agentAccessProject(w, r)
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int32 `json:"expected_version"`
	}
	if authBody(w, r, &body) != nil || body.ExpectedVersion <= 0 {
		writeAuthError(w, h.log, authflow.ErrInvalid)
		return
	}
	issued, err := h.flow.IssueAgentAccess(r.Context(), raw, project, body.ExpectedVersion)
	if err != nil {
		writeAuthError(w, h.log, err)
		return
	}
	authJSON(w, issued)
}
