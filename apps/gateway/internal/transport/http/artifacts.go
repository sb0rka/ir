package httptransport

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

func (server *Server) CreateArtifactAnalysis(w http.ResponseWriter, r *http.Request, _ api.CreateArtifactAnalysisParams) {
	var body api.CreateArtifactAnalysisRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(body.Source) == "" || strings.TrimSpace(body.Artifact.Name) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "source and artifact name are required")
		return
	}
	if !server.sourceAllowed(r.Context(), body.Source) {
		server.writeServiceError(w, sourceForbidden(body.Source))
		return
	}
	hashes := domain.Hashes{}
	if body.Artifact.Hashes != nil {
		hashes = hashesFromAPI(*body.Artifact.Hashes)
	}
	analysis, err := server.service.AnalyzeArtifact(r.Context(), body.Source, capability.AnalyzeArtifactRequest{
		Name: body.Artifact.Name, MIME: stringValue(body.Artifact.Mime), Hashes: hashes,
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusAccepted, analysisToAPI(analysis))
}

func (server *Server) GetArtifactAnalysis(w http.ResponseWriter, r *http.Request, analysisID uuid.UUID, _ api.GetArtifactAnalysisParams) {
	sources, err := server.constrainSources(r.Context(), nil, domain.CapabilityArtifactAnalysis)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	analysis, err := server.service.GetAnalysis(r.Context(), sources, analysisID.String())
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	if !server.sourceAllowed(r.Context(), analysis.Provenance.Source) {
		server.writeServiceError(w, sourceForbidden(analysis.Provenance.Source))
		return
	}
	respondJSON(w, http.StatusOK, analysisToAPI(analysis))
}
