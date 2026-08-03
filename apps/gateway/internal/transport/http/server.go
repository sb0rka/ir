package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	coretransport "github.com/sb0rka/sb0rka/packages/core/transport"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/application"
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const (
	projectIDHeader = "X-Project-ID"
	maxRequestBody  = 1 << 20
)

type projectIDKey struct{}

type Server struct {
	service        *application.Service
	log            *slog.Logger
	projectSources map[string]map[string]bool
}

func NewHandler(cfg config.Config, log *slog.Logger, service *application.Service) http.Handler {
	server := &Server{service: service, log: log, projectSources: cfg.ProjectSources}
	generated := http.NewServeMux()
	api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		BaseRouter: generated,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
	})

	protected := coretransport.Chain(http.Handler(generated), projectScope())
	if !cfg.Auth.Disabled {
		protected = coretransport.Auth(coretransport.AuthConfig{
			PublicKey: cfg.Auth.PublicKey,
			Issuer:    cfg.Auth.Issuer,
			Audience:  cfg.Auth.Audience,
			Kid:       cfg.Auth.Kid,
			Typ:       cfg.Auth.Typ,
		}, func(w http.ResponseWriter, _ *http.Request) {
			respondError(w, http.StatusUnauthorized, "unauthorized", "valid bearer token is required")
		})(protected)
	}

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", server.Healthz)
	root.HandleFunc("GET /readyz", server.Readyz)
	root.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	root.HandleFunc("GET /swagger", serveSwagger)
	root.Handle("/api/", protected)

	return coretransport.Chain(root,
		requestLogger(log),
		coretransport.Recover(log, func(w http.ResponseWriter, _ *http.Request) {
			respondError(w, http.StatusInternalServerError, "internal", "internal server error")
		}),
		cors(cfg.Server.CORSWhitelist),
	)
}

func (server *Server) Healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, api.Health{Status: api.Ok})
}

func (server *Server) Readyz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, api.Health{Status: api.Ok})
}

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

func (server *Server) SearchEvents(w http.ResponseWriter, r *http.Request, _ api.SearchEventsParams) {
	var body api.SearchEventsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request, err := searchEventsRequest(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	request.Sources, err = server.constrainSources(r.Context(), request.Sources)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchEvents(r.Context(), request)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchEventsResponse{
		Events:       eventsToAPI(result.Events),
		Entities:     entitiesToAPI(result.Entities),
		Relations:    relationsToAPI(result.Relations),
		SourceErrors: sourceErrorsToAPI(result.SourceErrors),
	}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

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
	result, err := server.service.LookupEntity(r.Context(), application.LookupEntityRequest{
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
	analysis, err := server.service.GetAnalysis(r.Context(), analysisID.String())
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

func (server *Server) SearchEndpoints(w http.ResponseWriter, r *http.Request, _ api.SearchEndpointsParams) {
	var body api.SearchEndpointsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateLimit(body.Limit); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sources, err := server.constrainSources(r.Context(), valueOrEmpty(body.Sources))
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	result, err := server.service.SearchEndpoints(r.Context(), application.SearchEndpointsRequest{
		Sources: sources, Query: stringValue(body.Query), Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	})
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := api.SearchEndpointsResponse{Items: endpointsToAPI(result.Items), SourceErrors: sourceErrorsToAPI(result.SourceErrors)}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	respondJSON(w, http.StatusOK, response)
}

func (server *Server) ListResponseActions(w http.ResponseWriter, r *http.Request, source, externalID string, _ api.ListResponseActionsParams) {
	if !server.sourceAllowed(r.Context(), source) {
		server.writeServiceError(w, sourceForbidden(source))
		return
	}
	items, err := server.service.ListResponseActions(r.Context(), source, externalID)
	if err != nil {
		server.writeServiceError(w, err)
		return
	}
	response := make([]api.ResponseAction, 0, len(items))
	for _, item := range items {
		response = append(response, api.ResponseAction{Code: item.Code, Title: item.Title, Destructive: item.Destructive, Enabled: item.Enabled})
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": response})
}

func (server *Server) writeServiceError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal", "internal server error"
	var requestErr *domain.RequestError
	switch {
	case errors.As(err, &requestErr):
		status, code, message = http.StatusBadRequest, requestErr.Code, requestErr.Message
		if requestErr.Code == "source_not_found" {
			status = http.StatusNotFound
		}
		if requestErr.Code == "source_forbidden" {
			status = http.StatusForbidden
		}
		if requestErr.Code == "artifact_not_in_scenario" {
			status = http.StatusUnprocessableEntity
		}
	case errors.Is(err, domain.ErrUnsupportedCapability):
		status, code, message = http.StatusUnprocessableEntity, "unsupported_capability", err.Error()
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, domain.ErrAllSourcesFailed):
		status, code, message = http.StatusBadGateway, "all_sources_failed", "all selected sources failed"
	case errors.Is(err, context.DeadlineExceeded):
		status, code, message = http.StatusGatewayTimeout, "timeout", "request timed out"
	default:
		server.log.Error("request_failed", "error", err)
	}
	respondError(w, status, code, message)
}

func (server *Server) constrainSources(ctx context.Context, requested []string) ([]string, error) {
	allowed, restricted := server.projectSources[projectIDFromContext(ctx)]
	if !restricted {
		return requested, nil
	}
	if len(requested) == 0 {
		result := make([]string, 0, len(allowed))
		for source := range allowed {
			result = append(result, source)
		}
		sort.Strings(result)
		return result, nil
	}
	for _, source := range requested {
		if !allowed[source] {
			return nil, sourceForbidden(source)
		}
	}
	return requested, nil
}

func (server *Server) sourceAllowed(ctx context.Context, source string) bool {
	allowed, restricted := server.projectSources[projectIDFromContext(ctx)]
	return !restricted || allowed[source]
}

func sourceForbidden(source string) error {
	return &domain.RequestError{Code: "source_forbidden", Message: fmt.Sprintf("source %q is not allowed for this project", source)}
}

func projectIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(projectIDKey{}).(string)
	return value
}

func projectScope() coretransport.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := strings.TrimSpace(r.Header.Get(projectIDHeader))
			if !validProjectID(projectID) {
				respondError(w, http.StatusBadRequest, "invalid_project_id", "X-Project-ID must be 10-12 lowercase hexadecimal characters")
				return
			}
			ctx := context.WithValue(r.Context(), projectIDKey{}, projectID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestLogger(log *slog.Logger) coretransport.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &coretransport.Recorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			fields := []any{"method", r.Method, "path", r.URL.Path, "status", recorder.Status(), "duration_ms", time.Since(started).Milliseconds()}
			if projectID := strings.TrimSpace(r.Header.Get(projectIDHeader)); validProjectID(projectID) {
				fields = append(fields, "project_id", projectID)
			}
			log.Info("request", fields...)
		})
	}
}

func validProjectID(value string) bool {
	if len(value) < 10 || len(value) > 12 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func cors(whitelist map[string]bool) coretransport.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wildcard := whitelist["*"]
			origin := r.Header.Get("Origin")
			allowed := wildcard || whitelist[origin]
			if !wildcard {
				w.Header().Add("Vary", "Origin")
			}
			if allowed {
				if wildcard {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Project-ID")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(api.OpenAPI)
}

func serveSwagger(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(api.SwaggerHTML))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON value")
	}
	return nil
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	body := api.ErrorEnvelope{}
	body.Error.Code = code
	body.Error.Message = message
	respondJSON(w, status, body)
}

func searchEventsRequest(body api.SearchEventsRequest) (application.SearchEventsRequest, error) {
	if err := validateLimit(body.Limit); err != nil {
		return application.SearchEventsRequest{}, err
	}
	request := application.SearchEventsRequest{
		Sources: valueOrEmpty(body.Sources), Query: stringValue(body.Query), Limit: intValue(body.Limit), Cursor: stringValue(body.Cursor),
	}
	if body.TimeRange != nil {
		if body.TimeRange.To.Before(body.TimeRange.From) {
			return application.SearchEventsRequest{}, fmt.Errorf("time_range.to must not precede time_range.from")
		}
		request.TimeFrom, request.TimeTo = body.TimeRange.From, body.TimeRange.To
	}
	if body.Entities != nil {
		request.Entities = make([]domain.EntityRef, 0, len(*body.Entities))
		for _, entity := range *body.Entities {
			if strings.TrimSpace(entity.Type) == "" || strings.TrimSpace(entity.Value) == "" {
				return application.SearchEventsRequest{}, fmt.Errorf("entity type and value are required")
			}
			request.Entities = append(request.Entities, domain.EntityRef{Type: entity.Type, Value: entity.Value})
		}
	}
	return request, nil
}

func validateLimit(limit *int) error {
	if limit != nil && (*limit < 1 || *limit > 100) {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	return nil
}

func sourceToAPI(value domain.Source) api.Source {
	capabilities := make([]api.Capability, 0, len(value.Capabilities))
	for _, item := range value.Capabilities {
		capabilities = append(capabilities, api.Capability(item))
	}
	return api.Source{Code: value.Code, Name: value.Name, Kind: api.SourceKind(value.Kind), Mode: api.SourceMode(value.Mode), Status: api.SourceStatus(value.Status), Capabilities: capabilities}
}

func provenanceToAPI(value domain.Provenance) api.Provenance {
	result := api.Provenance{Source: value.Source, ExternalId: value.ExternalID, FetchedAt: value.FetchedAt}
	if value.SourceURL != "" {
		result.SourceUrl = &value.SourceURL
	}
	return result
}

func entitiesToAPI(values []domain.Entity) []api.Entity {
	result := make([]api.Entity, 0, len(values))
	for _, value := range values {
		provenance := make([]api.Provenance, 0, len(value.Provenance))
		for _, item := range value.Provenance {
			provenance = append(provenance, provenanceToAPI(item))
		}
		result = append(result, api.Entity{Id: value.ID, Type: value.Type, Value: value.Value, Attributes: nonNilMap(value.Attributes), Provenance: provenance})
	}
	return result
}

func eventsToAPI(values []domain.Event) []api.Event {
	result := make([]api.Event, 0, len(values))
	for _, value := range values {
		result = append(result, api.Event{Id: value.ID, Type: value.Type, Title: value.Title, Severity: api.EventSeverity(value.Severity), OccurredAt: value.OccurredAt, EntityIds: value.EntityIDs, Attributes: nonNilMap(value.Attributes), Provenance: provenanceToAPI(value.Provenance)})
	}
	return result
}

func relationsToAPI(values []domain.Relation) []api.Relation {
	result := make([]api.Relation, 0, len(values))
	for _, value := range values {
		result = append(result, api.Relation{Id: value.ID, Type: value.Type, SourceEntityId: value.SourceEntityID, TargetEntityId: value.TargetEntityID, OccurredAt: value.OccurredAt, Provenance: provenanceToAPI(value.Provenance)})
	}
	return result
}

func artifactToAPI(value domain.Artifact) api.Artifact {
	result := api.Artifact{Id: value.ID, Name: value.Name, Hashes: hashesToAPI(value.Hashes)}
	if value.MIME != "" {
		result.Mime = &value.MIME
	}
	if value.Size > 0 {
		result.Size = &value.Size
	}
	return result
}

func analysisToAPI(value domain.Analysis) api.Analysis {
	artifacts := make([]api.Artifact, 0, len(value.Artifacts))
	for _, artifact := range value.Artifacts {
		artifacts = append(artifacts, artifactToAPI(artifact))
	}
	result := api.Analysis{Id: value.ID, Status: api.AnalysisStatus(value.Status), Artifact: artifactToAPI(value.Artifact), Verdict: verdictToAPI(value.Verdict), Artifacts: artifacts, Provenance: provenanceToAPI(value.Provenance)}
	if value.Attributes != nil {
		result.Attributes = &value.Attributes
	}
	return result
}

func endpointsToAPI(values []domain.Endpoint) []api.Endpoint {
	result := make([]api.Endpoint, 0, len(values))
	for _, value := range values {
		item := api.Endpoint{Id: value.ID, ExternalId: value.ExternalID, Hostname: value.Hostname, Status: api.EndpointStatus(value.Status), Attributes: nonNilMap(value.Attributes), Provenance: provenanceToAPI(value.Provenance)}
		if len(value.IPAddresses) > 0 {
			addresses := append([]string(nil), value.IPAddresses...)
			item.IpAddresses = &addresses
		}
		result = append(result, item)
	}
	return result
}

func sourceErrorsToAPI(values []domain.SourceError) []api.SourceError {
	result := make([]api.SourceError, 0, len(values))
	for _, value := range values {
		result = append(result, api.SourceError{Source: value.Source, Code: value.Code, Message: value.Message, Retryable: value.Retryable})
	}
	return result
}

func verdictsToAPI(values []domain.Verdict) []api.Verdict {
	result := make([]api.Verdict, 0, len(values))
	for _, value := range values {
		result = append(result, verdictToAPI(value))
	}
	return result
}

func verdictToAPI(value domain.Verdict) api.Verdict {
	return api.Verdict{Value: api.VerdictValue(value.Value), Confidence: value.Confidence, Labels: nonNilSlice(value.Labels), Provider: value.Provider}
}

func hashesFromAPI(value api.Hashes) domain.Hashes {
	return domain.Hashes{MD5: stringValue(value.Md5), SHA1: stringValue(value.Sha1), SHA256: stringValue(value.Sha256)}
}

func hashesToAPI(value domain.Hashes) api.Hashes {
	return api.Hashes{Md5: stringPointer(value.MD5), Sha1: stringPointer(value.SHA1), Sha256: stringPointer(value.SHA256)}
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func valueOrEmpty[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}

var _ api.ServerInterface = (*Server)(nil)
