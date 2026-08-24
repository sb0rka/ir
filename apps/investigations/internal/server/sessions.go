package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/sessions"
)

func (s *Server) ListSessions(ctx context.Context, request sessions.ListSessionsRequestObject) (sessions.ListSessionsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "limit must be between 1 and 200")
	}
	cursor, err := decodeCursor(request.Params.Cursor)
	if err != nil {
		return nil, err
	}
	filter := model.ObjectFilter{Cursor: cursor, Limit: limit + 1}
	if request.Params.Severity != nil {
		value := string(*request.Params.Severity)
		filter.Severity = &value
	}
	if request.Params.ContextStatus != nil {
		value := string(*request.Params.ContextStatus)
		filter.ContextStatus = &value
	}
	items, err := s.db.InvestigationSessions(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := sessions.SessionPage{Sessions: make([]sessions.NetworkSession, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.AttachedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertNetworkSession(item)
		if err != nil {
			return nil, err
		}
		page.Sessions = append(page.Sessions, converted)
	}
	return sessions.ListSessions200JSONResponse(page), nil
}

func (s *Server) GetSession(ctx context.Context, request sessions.GetSessionRequestObject) (sessions.GetSessionResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetSession(ctx, scope.ProjectID, request.SessionId.String())
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertNetworkSession(item)
	if err != nil {
		return nil, err
	}
	return sessions.GetSession200JSONResponse(out), nil
}

func (s *Server) DetachSession(ctx context.Context, request sessions.DetachSessionRequestObject) (sessions.DetachSessionResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DetachSession(ctx, scope.ProjectID, request.InvestigationId.String(), request.SessionId.String()); err != nil {
		return nil, storeError(err)
	}
	return sessions.DetachSession204Response{}, nil
}

func convertNetworkSession(item model.NetworkSession) (sessions.NetworkSession, error) {
	var normalized, provenance any
	var contextErrors []model.GatewayContextError
	if err := json.Unmarshal(item.Normalized, &normalized); err != nil {
		return sessions.NetworkSession{}, err
	}
	if err := json.Unmarshal(item.Provenance, &provenance); err != nil {
		return sessions.NetworkSession{}, err
	}
	if err := json.Unmarshal(item.ContextErrors, &contextErrors); err != nil {
		return sessions.NetworkSession{}, err
	}
	payload := map[string]any{
		"id": item.ID, "ref": item.Ref, "title": item.Title, "severity": item.Severity,
		"started_at": item.StartedAt, "ended_at": item.EndedAt, "source_ref": item.SourceRef, "fetched_at": item.FetchedAt,
		"normalized_snapshot": normalized, "provenance": provenance,
		"context_status": item.ContextStatus, "context_errors": contextErrors,
		"created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
	}
	if len(item.InvestigationIDs) > 0 {
		payload["investigation_ids"] = item.InvestigationIDs
	}
	if !item.AttachedAt.IsZero() {
		payload["directly_added"] = item.Direct
		payload["derived"] = item.Derived
		payload["attached_at"] = item.AttachedAt
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return sessions.NetworkSession{}, err
	}
	var out sessions.NetworkSession
	if err := json.Unmarshal(raw, &out); err != nil {
		return sessions.NetworkSession{}, err
	}
	return out, nil
}
