package transport

import (
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/authctx"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
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

func recoverMiddleware(log *slog.Logger) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "recover", rec, "path", r.URL.Path)
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
			next.ServeHTTP(w, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration_ms", time.Since(started).Milliseconds())
		})
	}
}

func corsMiddleware(cfg config.ServerConfig) middlewareChain {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if cfg.CORSAllowedAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && cfg.CORSWhitelist[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authMiddleware проверяет токен платформы публичным ключом и кладёт личность
// в контекст. Роли подтягиваются резолвером — он ходит в role_bindings.
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

			if roles != nil {
				resolved, err := roles.Resolve(r.Context(), identity.SubjectID)
				if err != nil {
					httperr.Write(w, log, err)
					return
				}
				identity.ProjectID = resolved.ProjectID
				identity.Roles = resolved.Roles
			}
			if len(identity.Roles) == 0 && cfg.DefaultRole != "" {
				identity.Roles = []string{cfg.DefaultRole}
			}
			for _, subject := range cfg.BootstrapAdminSubjects {
				if subject == identity.SubjectID {
					identity.Roles = append(identity.Roles, "admin")
					break
				}
			}

			next.ServeHTTP(w, r.WithContext(authctx.With(r.Context(), identity)))
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

func verify(raw string, cfg config.AuthConfig) (authctx.Identity, error) {
	if len(cfg.AccessTokenPublicKey) == 0 {
		return authctx.Identity{}, httperr.New(
			http.StatusUnauthorized, httperr.CodeUnauthorized, "token verification is not configured")
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
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(cfg.AccessTokenIssuer),
		jwt.WithAudience(cfg.AccessTokenAudience),
	)
	if err != nil {
		return authctx.Identity{}, httperr.ErrUnauthorized
	}
	if claims.Subject == "" || claims.SessionID == "" {
		return authctx.Identity{}, httperr.ErrUnauthorized
	}

	return authctx.Identity{
		SubjectID:   claims.Subject,
		SubjectKind: claims.SubjectKind,
		SessionID:   claims.SessionID,
	}, nil
}
