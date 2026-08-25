package httptransport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/api"
)

func (server *Server) ListSources(w http.ResponseWriter, r *http.Request, params api.ListSourcesParams) {
	allowed, err := server.allowedSources(r.Context())
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	refresh := params.Refresh != nil && *params.Refresh
	sources := server.service.ListSources(r.Context(), projectAccess(r), allowed, refresh)
	items := make([]api.Source, 0, len(sources))
	for _, source := range sources {
		items = append(items, sourceToAPI(source))
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": items})
}
