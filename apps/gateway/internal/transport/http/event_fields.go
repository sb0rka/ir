package httptransport

import (
	"net/http"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
)

func (server *Server) ListEventFields(w http.ResponseWriter, r *http.Request, _ api.ListEventFieldsParams) {
	allowed, err := server.allowedSources(r.Context())
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.EventFields(r.Context(), allowed)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}
func commonFilterFromAPI(value *api.CommonEventFilter) (fieldmap.CommonFilter, error) {
	filter := fieldmap.CommonFilter{}
	if value != nil {
		if value.Hosts != nil {
			filter.Hosts = *value.Hosts
		}
		if value.Ips != nil {
			filter.IPs = *value.Ips
		}
	}
	return fieldmap.Normalize(filter)
}
