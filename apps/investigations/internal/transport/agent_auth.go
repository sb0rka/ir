package transport

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
)

const (
	agentTokenAudience = "ir-mcp"
	agentTokenType     = "agent+jwt"
)

type investigationAgentClaims struct {
	SessionID       string `json:"sid"`
	SubjectKind     string `json:"sk"`
	ClientID        string `json:"client_id"`
	ProjectID       string `json:"project_id"`
	InvestigationID string `json:"investigation_id"`
	Scope           string `json:"scope"`
	jwt.RegisteredClaims
}

func mcpAuthMiddleware(cfg config.ServerConfig, log *slog.Logger) middlewareChain {
	return func(next http.Handler) http.Handler {
		human := authMiddleware(cfg, log)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := bearerToken(r.Header.Get("Authorization"))
			if err != nil {
				httperr.Write(w, log, err)
				return
			}
			if !looksLikeAgentToken(raw) {
				human.ServeHTTP(w, r)
				return
			}
			claims, err := verifyAgentToken(raw, cfg.Auth)
			if err != nil {
				httperr.Write(w, log, httperr.ErrUnauthorized)
				return
			}
			ctx := socctx.WithScope(r.Context(), socctx.Scope{ProjectID: claims.ProjectID})
			ctx = socctx.WithAgentAuthorization(ctx, socctx.AgentAuthorization{
				InvestigationID: claims.InvestigationID,
				Scope:           claims.Scope,
				Token:           raw,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func verifyAgentToken(raw string, cfg config.AuthConfig) (investigationAgentClaims, error) {
	if len(cfg.AccessTokenPublicKey) == 0 || cfg.AccessTokenIssuer == "" || cfg.AccessTokenKid == "" {
		return investigationAgentClaims{}, errors.New("agent verifier is not configured")
	}
	claims := investigationAgentClaims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodEdDSA || token.Header["alg"] != jwt.SigningMethodEdDSA.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		kid, _ := token.Header["kid"].(string)
		typ, _ := token.Header["typ"].(string)
		if kid != cfg.AccessTokenKid || typ != agentTokenType {
			return nil, errors.New("unexpected token header")
		}
		return cfg.AccessTokenPublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer(cfg.AccessTokenIssuer),
		jwt.WithAudience(agentTokenAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || token == nil || !token.Valid {
		return investigationAgentClaims{}, errors.New("invalid agent token")
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != agentTokenAudience ||
		claims.Subject == "" || claims.SessionID == "" || claims.SubjectKind == "" ||
		claims.ID == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil ||
		!validProjectID(claims.ProjectID) || strings.TrimSpace(claims.Scope) == "" {
		return investigationAgentClaims{}, errors.New("invalid agent claims")
	}
	id, err := uuid.Parse(claims.InvestigationID)
	if err != nil || id == uuid.Nil {
		return investigationAgentClaims{}, errors.New("invalid investigation claim")
	}
	return claims, nil
}

func tokenHeaderType(raw string) string {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return ""
	}
	encoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	var header struct {
		Type string `json:"typ"`
	}
	if json.Unmarshal(encoded, &header) != nil {
		return ""
	}
	return header.Type
}

func looksLikeAgentToken(raw string) bool {
	if tokenHeaderType(raw) == agentTokenType {
		return true
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	encoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var payload struct {
		Audience        any    `json:"aud"`
		ProjectID       string `json:"project_id"`
		InvestigationID string `json:"investigation_id"`
	}
	if json.Unmarshal(encoded, &payload) != nil {
		return false
	}
	if payload.ProjectID != "" || payload.InvestigationID != "" {
		return true
	}
	switch audience := payload.Audience.(type) {
	case string:
		return audience == agentTokenAudience
	case []any:
		for _, value := range audience {
			if value == agentTokenAudience {
				return true
			}
		}
	}
	return false
}
