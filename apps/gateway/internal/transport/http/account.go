package httptransport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/api"
)

func (server *Server) GetSourceAccountUserinfo(w http.ResponseWriter, r *http.Request, source string, _ api.GetSourceAccountUserinfoParams) {
	if !server.sourceAllowed(r.Context(), source) {
		server.writeServiceError(w, sourceForbidden(source))
		return
	}
	userinfo, err := server.service.GetAccountUserinfo(r.Context(), projectAccess(r), source)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, api.AccountUserinfo{SourceCode: userinfo.SourceCode, UserName: userinfo.UserName})
}
