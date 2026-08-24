package httptransport

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

func (server *Server) SearchFindings(w http.ResponseWriter, r *http.Request, _ api.SearchFindingsParams) {
	var body api.SearchFindingsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateLimit(body.Limit); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	timeRange, err := objectTimeRange(body.TimeRange.From, body.TimeRange.To)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sources, err := server.constrainSources(r.Context(), valueOrEmpty(body.Sources), domain.CapabilityFindings)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	kinds := make([]string, 0)
	if body.Kinds != nil {
		kinds = make([]string, 0, len(*body.Kinds))
		seen := make(map[string]struct{}, len(*body.Kinds))
		for _, kind := range *body.Kinds {
			value := string(kind)
			switch value {
			case "siem_incident", "siem_correlation", "nad_attack":
			default:
				respondError(w, http.StatusBadRequest, "bad_request", "unsupported finding kind")
				return
			}
			if _, duplicate := seen[value]; duplicate {
				respondError(w, http.StatusBadRequest, "bad_request", "kinds must be unique")
				return
			}
			seen[value] = struct{}{}
			kinds = append(kinds, value)
		}
	}
	result, err := server.service.SearchFindings(r.Context(), projectAccess(r), service.SearchFindingsRequest{
		Sources: sources, Kinds: kinds, TimeRange: timeRange, Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchFindingsResponse{
		Findings: findingsToAPI(result.Findings), SourceStates: sourceStatesToAPI(result.SourceStates), SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) GetFinding(w http.ResponseWriter, r *http.Request, source string, kind api.GetFindingParamsKind, externalID string, params api.GetFindingParams) {
	if !server.sourceAllowed(r.Context(), source) {
		server.writeServiceError(w, sourceForbidden(source))
		return
	}
	if !server.service.Supports(source, domain.CapabilityFindings) {
		server.writeServiceError(w, fmt.Errorf("%w: source %q does not support findings", domain.ErrUnsupportedCapability, source))
		return
	}
	instance := stringValue(params.SourceInstance)
	if err := validateFindingRef(server, source, string(kind), instance, externalID); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	timeRange, err := objectTimeRange(params.From, params.To)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	finding, resolution, err := server.service.GetFinding(r.Context(), projectAccess(r), domain.SourceObjectRef{
		SourceCode: source, SourceInstance: instance, RecordType: string(kind), ExternalID: externalID, TimeRange: timeRange,
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"finding": findingsToAPI([]domain.Finding{finding})[0], "resolution": resolutionsToAPI([]domain.ObjectResolution{resolution})[0]})
}

func validateFindingRef(server *Server, source, kind, instance, externalID string) error {
	if strings.TrimSpace(externalID) == "" || len(externalID) > 512 {
		return fmt.Errorf("external_id is required and must not exceed 512 characters")
	}
	switch kind {
	case "siem_incident", "siem_correlation":
		if source != config.PTMaxPatrolSIEM || strings.TrimSpace(instance) != "" {
			return fmt.Errorf("SIEM findings require source %q and no source_instance", config.PTMaxPatrolSIEM)
		}
	case "nad_attack":
		if source != config.PTNAD || !server.sourceInstanceAllowed(source, instance) {
			return fmt.Errorf("NAD finding source_instance is not configured")
		}
	default:
		return fmt.Errorf("unsupported finding kind %q", kind)
	}
	return nil
}

func objectTimeRange(from, to time.Time) (domain.TimeRange, error) {
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return domain.TimeRange{}, fmt.Errorf("time_range must satisfy from < to")
	}
	return domain.TimeRange{From: from.UTC(), To: to.UTC()}, nil
}
