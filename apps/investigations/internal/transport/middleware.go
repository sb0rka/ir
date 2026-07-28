package transport

import (
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	coreauthctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

// accessTokenClaims повторяет формат токена платформы sb0rka.
type accessTokenClaims struct {
	SessionID   string `json:"sid"`
	SubjectKind string `json:"sk"`
	jwt.RegisteredClaims
}

type middlewareChain func(http.Handler) http.Handler

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
					allowHeaders += ", Authorization"
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
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

func authMiddleware(cfg config.ServerConfig, log *slog.Logger, roles RoleResolver) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			var scope socctx.Scope
			if roles != nil {
				resolved, err := roles.Resolve(r.Context(), identity.SubjectID)
				if err != nil {
					httperr.Write(w, log, err)
					return
				}
				scope.ProjectID = resolved.ProjectID
				scope.Roles = resolved.Roles
			}
			if len(scope.Roles) == 0 && cfg.DefaultRole != "" {
				scope.Roles = []string{cfg.DefaultRole}
			}
			// Deny-by-default: валидная подпись — ещё не право работать
			// в продукте. Без единой роли доступа нет, иначе отзыв биндингов
			// не отзывал бы доступ.
			if len(scope.Roles) == 0 {
				httperr.Write(w, log, httperr.ErrForbidden)
				return
			}

			ctx := coreauthctx.WithIdentity(r.Context(), identity)
			ctx = socctx.WithScope(ctx, scope)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", httperr.ErrUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", httperr.ErrUnauthorized
	}
	return token, nil
}

func verify(raw string, cfg config.AuthConfig) (coreauthctx.Identity, error) {
	if len(cfg.AccessTokenPublicKey) == 0 {
		return coreauthctx.Identity{}, httperr.New(
			http.StatusUnauthorized, httperr.CodeUnauthorized, "token verification is not configured")
	}

	// Проверки iss/aud навешиваются только если заданы: пустое значение
	// в jwt.WithIssuer/WithAudience требует пустого поля в токене, то есть
	// отклоняет вообще всё.
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithExpirationRequired(),
	}
	if cfg.AccessTokenIssuer != "" {
		opts = append(opts, jwt.WithIssuer(cfg.AccessTokenIssuer))
	}
	if cfg.AccessTokenAudience != "" {
		opts = append(opts, jwt.WithAudience(cfg.AccessTokenAudience))
	}

	claims := &accessTokenClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if cfg.AccessTokenKid != "" {
			kid, _ := token.Header["kid"].(string)
			if kid != cfg.AccessTokenKid {
				return nil, jwt.ErrTokenUnverifiable
			}
		}
		if cfg.AccessTokenTyp != "" {
			typ, _ := token.Header["typ"].(string)
			if typ != cfg.AccessTokenTyp {
				return nil, jwt.ErrTokenUnverifiable
			}
		}
		return ed25519.PublicKey(cfg.AccessTokenPublicKey), nil
	}, opts...)
	if err != nil {
		return coreauthctx.Identity{}, httperr.ErrUnauthorized
	}
	if claims.Subject == "" || claims.SessionID == "" {
		return coreauthctx.Identity{}, httperr.ErrUnauthorized
	}

	return coreauthctx.Identity{
		SubjectID:   claims.Subject,
		SubjectKind: claims.SubjectKind,
		SessionID:   claims.SessionID,
		JTI:         claims.ID,
	}, nil
}
