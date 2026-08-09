package httptransport

import (
	"net/http"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

func (server *Server) LookupEntity(w http.ResponseWriter, r *http.Request, _ api.LookupEntityParams) {
	var body api.LookupEntityRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(body.Entity.Type) == "" || strings.TrimSpace(body.Entity.Value) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "entity type and value are required")
		return
	}
	sources, err := server.constrainSources(r.Context(), valueOrEmpty(body.Sources))
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.LookupEntity(r.Context(), service.LookupEntityRequest{
		Sources: sources,
		Entity:  domain.EntityRef{Type: body.Entity.Type, Value: body.Entity.Value},
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, api.LookupEntityResponse{
		Entities:     entitiesToAPI(result.Entities),
		Relations:    relationsToAPI(result.Relations),
		Verdicts:     verdictsToAPI(result.Verdicts),
		SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	})
}
