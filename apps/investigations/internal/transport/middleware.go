package transport

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	coreauth "github.com/sb0rka/sb0rka/packages/core/auth"
	coreauthctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

type middlewareChain func(http.Handler) http.Handler

const projectIDHeader = "X-Project-ID"

func chain(h http.Handler, middlewares ...middlewareChain) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// recorder нужен двоим: логу — чтобы писать статус, recover — чтобы понимать,
// можно ли ещё отдать конверт ошибки.
type recorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rec *recorder) WriteHeader(status int) {
	if rec.written {
		return
	}
	rec.status = status
	rec.written = true
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *recorder) Write(b []byte) (int, error) {
	if !rec.written {
		rec.WriteHeader(http.StatusOK)
	}
	return rec.ResponseWriter.Write(b)
}

func recoverMiddleware(log *slog.Logger) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec, _ := w.(*recorder)
			defer func() {
				if p := recover(); p != nil {
					log.Error("panic", "recover", p, "path", r.URL.Path)
					// После первой записи дописать корректный ответ уже нельзя:
					// второй WriteHeader даёт «superfluous WriteHeader» в логе
					// вместо самой паники. Рвём соединение — клиент увидит
					// обрыв, а не притворно успешный ответ.
					if rec != nil && rec.written {
						panic(http.ErrAbortHandler)
					}
					httperr.Write(w, log, httperr.New(
						http.StatusInternalServerError, httperr.CodeInternal, "internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func loggerMiddleware(log *slog.Logger) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(started).Milliseconds())
		})
	}
}

// corsMiddleware повторяет поведение apps/api платформы, чтобы фронт видел
// одинаковые заголовки от всех сервисов. Authorization и credentials идут
// только к явно разрешённому источнику: с «*» браузер их всё равно отвергнет.
func corsMiddleware(cfg config.ServerConfig) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var allowedOrigin string
			if cfg.CORSWhitelist["*"] {
				allowedOrigin = "*"
			} else if origin := r.Header.Get("Origin"); cfg.CORSWhitelist[origin] {
				allowedOrigin = origin
			}

			// Vary ставится и когда источник не разрешён: ответ всё равно
			// зависит от Origin, и без этого кэш отдал бы чужому источнику
			// ответ, собранный для разрешённого.
			if !cfg.CORSWhitelist["*"] {
				w.Header().Add("Vary", "Origin")
			}

			if allowedOrigin != "" {
				allowHeaders := "Content-Type"
				if allowedOrigin != "*" {
					allowHeaders += ", Authorization, " + projectIDHeader
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				// Без этого префлайт летит перед каждым запросом.
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

func authMiddleware(cfg config.ServerConfig, log *slog.Logger) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Demo-режим сохраняет тот же project scope, но не проверяет подпись.
			// Если bearer передан, он нужен только для project-scoped Sb0rka API.
			if cfg.Auth.Disabled {
				projectID := strings.TrimSpace(r.Header.Get(projectIDHeader))
				if !validProjectID(projectID) {
					httperr.Write(w, log, httperr.BadRequest("X-Project-ID must be 10-12 lowercase hexadecimal characters"))
					return
				}
				if token, err := coreauth.ParseBearerToken(r.Header.Get("Authorization")); err == nil {
					ctx = socctx.WithBearer(ctx, token)
				}
				ctx = socctx.WithScope(ctx, socctx.Scope{ProjectID: projectID})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			raw, err := bearerToken(r.Header.Get("Authorization"))
			if err != nil {
				httperr.Write(w, log, err)
				return
			}

			identity, err := verify(raw, cfg.Auth)
			if err != nil {
				httperr.Write(w, log, err)
				return
			}
			projectID := strings.TrimSpace(r.Header.Get(projectIDHeader))
			if !validProjectID(projectID) {
				httperr.Write(w, log, httperr.BadRequest("X-Project-ID must be 10-12 lowercase hexadecimal characters"))
				return
			}

			ctx = coreauthctx.WithIdentity(ctx, identity)
			ctx = socctx.WithBearer(ctx, raw)
			ctx = socctx.WithScope(ctx, socctx.Scope{ProjectID: projectID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, error) {
	token, err := coreauth.ParseBearerToken(header)
	if err != nil {
		return "", httperr.ErrUnauthorized
	}
	return token, nil
}

func verify(raw string, cfg config.AuthConfig) (coreauthctx.Identity, error) {
	identity, err := coreauth.VerifyAccessToken(raw, coreauth.VerificationConfig{
		PublicKey: cfg.AccessTokenPublicKey,
		KeyID:     cfg.AccessTokenKid,
		TokenType: cfg.AccessTokenTyp,
		Issuer:    cfg.AccessTokenIssuer,
		Audience:  cfg.AccessTokenAudience,
	})
	if err != nil {
		return coreauthctx.Identity{}, httperr.ErrUnauthorized
	}

	return coreauthctx.Identity{
		SubjectID:   identity.SubjectID,
		SubjectKind: identity.SubjectKind,
		SessionID:   identity.SessionID,
		JTI:         identity.JTI,
		ClientID:    identity.ClientID,
	}, nil
}

func validProjectID(projectID string) bool {
	if len(projectID) < 10 || len(projectID) > 12 {
		return false
	}
	for _, char := range projectID {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
