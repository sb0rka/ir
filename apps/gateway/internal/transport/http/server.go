package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/sb0rka/ir/packages/common/irconfig"
	coretransport "github.com/sb0rka/sb0rka/packages/core/transport"

	"github.com/sb0rka/ir/apps/gateway/api"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
)

const (
	projectIDHeader = "X-Project-ID"
	maxRequestBody  = 1 << 20
)

type projectAccessKey struct{}

type Server struct {
	service         *service.Service
	log             *slog.Logger
	sourceInstances map[string]map[string]bool
}

func newGatewayServer(cfg config.Config, log *slog.Logger, service *service.Service) *Server {
	instances := make(map[string]map[string]bool)
	for source, sourceConfig := range cfg.Sources {
		if len(sourceConfig.InstanceIDs) == 0 {
			continue
		}
		instances[source] = make(map[string]bool, len(sourceConfig.InstanceIDs))
		for _, storeID := range sourceConfig.InstanceIDs {
			instances[source][storeID] = true
		}
	}
	return &Server{service: service, log: log, sourceInstances: instances}
}

func NewHandler(cfg config.Config, log *slog.Logger, service *service.Service, options ...HandlerOption) http.Handler {
	server := newGatewayServer(cfg, log, service)
	var dependencies handlerDependencies
	for _, option := range options {
		option(&dependencies)
	}
	protected := gatewayAuthentication(cfg.Auth, dependencies.agentAccess)(apiHandler(server))

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", server.Healthz)
	root.HandleFunc("GET /ping", server.Ping)
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

// apiHandler is the project-scoped API without Gateway token checks.
// Tests call it directly; production always wraps it in gatewayAuthentication.
func apiHandler(server *Server) http.Handler {
	generated := http.NewServeMux()
	api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		BaseRouter: generated,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			respondError(w, http.StatusBadRequest, "bad_request", err.Error())
		},
	})
	return coretransport.Chain(http.Handler(generated), projectScope())
}

func (server *Server) Healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, api.Health{Status: api.Ok})
}

func (server *Server) Ping(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}

func (server *Server) writeServiceError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal", "internal server error"
	var requestErr *domain.RequestError
	switch {
	case errors.As(err, &requestErr):
		status, code, message = http.StatusBadRequest, requestErr.Code, requestErr.Message
		if requestErr.Code == "source_not_found" || requestErr.Code == "source_record_not_found" {
			status = http.StatusNotFound
		}
		if requestErr.Code == "source_forbidden" {
			status = http.StatusForbidden
		}
		if requestErr.Code == "source_unavailable" {
			status = http.StatusServiceUnavailable
		}
		if requestErr.Code == "source_auth_failed" {
			status = http.StatusBadGateway
		}
		if requestErr.Code == "provider_error" {
			status = http.StatusBadGateway
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
		var sourcesErr *service.AllSourcesError
		if errors.As(err, &sourcesErr) {
			// A predicate rejected by every adapter before reaching the vendor is a
			// client error; hiding its message behind 502 leaves agents guessing.
			for _, item := range sourcesErr.Items {
				if item.Code == "invalid_request" || item.Code == "invalid_source_request" {
					status, code, message = http.StatusBadRequest, item.Code, item.Message
					break
				}
			}
		}
	case errors.Is(err, context.DeadlineExceeded):
		status, code, message = http.StatusGatewayTimeout, "timeout", "request timed out"
	default:
		server.log.Error("request_failed", "error", err)
	}
	respondError(w, status, code, message)
}

func (server *Server) allowedSources(ctx context.Context) ([]string, error) {
	if server.service == nil {
		return []string{}, nil
	}
	return server.service.AvailableSources(ctx, projectAccessFromContext(ctx)), nil
}

func (server *Server) constrainSources(
	ctx context.Context,
	requested []string,
	capabilityName domain.Capability,
) ([]string, error) {
	allowedList, err := server.allowedSources(ctx)
	if err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		result := make([]string, 0, len(allowedList))
		for _, source := range allowedList {
			if server.service.Supports(source, capabilityName) {
				result = append(result, source)
			}
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("%w: no project source supports %s", domain.ErrUnsupportedCapability, capabilityName)
		}
		return result, nil
	}
	for _, source := range requested {
		if !slices.Contains(allowedList, source) {
			return nil, sourceForbidden(source)
		}
		if !server.service.Supports(source, capabilityName) {
			return nil, fmt.Errorf(
				"%w: source %q does not support %s",
				domain.ErrUnsupportedCapability,
				source,
				capabilityName,
			)
		}
	}
	return requested, nil
}

func (server *Server) sourceAllowed(ctx context.Context, source string) bool {
	return server.service != nil && server.service.SourceAvailable(ctx, projectAccessFromContext(ctx), source)
}

func (server *Server) sourceInstanceAllowed(source, instance string) bool {
	instances, constrained := server.sourceInstances[source]
	if !constrained {
		return strings.TrimSpace(instance) == ""
	}
	return instances[strings.TrimSpace(instance)]
}

func sourceForbidden(source string) error {
	return &domain.RequestError{
		Code:    "source_forbidden",
		Message: fmt.Sprintf("source %q is not allowed for this project", source),
	}
}

func projectAccessFromContext(ctx context.Context) service.ProjectAccess {
	value, _ := ctx.Value(projectAccessKey{}).(service.ProjectAccess)
	return value
}

func projectScope() coretransport.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := strings.TrimSpace(r.Header.Get(projectIDHeader))
			if !validProjectID(projectID) {
				respondError(
					w,
					http.StatusBadRequest,
					"invalid_project_id",
					"X-Project-ID must be 10-12 lowercase hexadecimal characters",
				)
				return
			}
			bearer, _ := coretransport.BearerToken(r.Header.Get(irconfig.TokenHeader))
			if agent, ok := r.Context().Value(agentAccessContextKey{}).(agentRequestAccess); ok {
				if agent.project != projectID {
					respondError(w, http.StatusUnauthorized, "unauthorized", "agent project does not match")
					return
				}
				bearer = agent.bearer
			}
			ctx := context.WithValue(r.Context(), projectAccessKey{}, service.ProjectAccess{ProjectID: projectID, Bearer: bearer})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func projectAccess(r *http.Request) service.ProjectAccess {
	return projectAccessFromContext(r.Context())
}

func requestLogger(log *slog.Logger) coretransport.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &coretransport.Recorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			fields := []any{
				"method",
				r.Method,
				"path",
				r.URL.Path,
				"status",
				recorder.Status(),
				"duration_ms",
				time.Since(started).Milliseconds(),
			}
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
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

func validateLimit(limit *int) error {
	if limit != nil && (*limit < 1 || *limit > 100) {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	return nil
}

var _ api.ServerInterface = (*Server)(nil)
