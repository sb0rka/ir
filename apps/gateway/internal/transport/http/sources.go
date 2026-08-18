package httptransport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/api"
)

func (server *Server) ListSources(w http.ResponseWriter, r *http.Request, _ api.ListSourcesParams) {
	items := make([]api.Source, 0, len(server.service.ListSources()))
	for _, source := range server.service.ListSources() {
		if !server.sourceAllowed(r.Context(), source.Code) {
			continue
		}
		items = append(items, sourceToAPI(source))
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}
