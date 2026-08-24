package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/findings"
)

func (s *Server) ListFindings(ctx context.Context, request findings.ListFindingsRequestObject) (findings.ListFindingsResponseObject, error) {
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
	if request.Params.RecordType != nil {
		value := string(*request.Params.RecordType)
		filter.RecordType = &value
	}
	if request.Params.Severity != nil {
		value := string(*request.Params.Severity)
		filter.Severity = &value
	}
	if request.Params.ContextStatus != nil {
		value := string(*request.Params.ContextStatus)
		filter.ContextStatus = &value
	}
	items, err := s.db.InvestigationFindings(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := findings.FindingPage{Findings: make([]findings.Finding, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.AttachedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertFinding(item)
		if err != nil {
			return nil, err
		}
		page.Findings = append(page.Findings, converted)
	}
	return findings.ListFindings200JSONResponse(page), nil
}

func (s *Server) GetFinding(ctx context.Context, request findings.GetFindingRequestObject) (findings.GetFindingResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetFinding(ctx, scope.ProjectID, request.FindingId.String())
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertFinding(item)
	if err != nil {
		return nil, err
	}
	return findings.GetFinding200JSONResponse(out), nil
}

func (s *Server) DetachFinding(ctx context.Context, request findings.DetachFindingRequestObject) (findings.DetachFindingResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DetachFinding(ctx, scope.ProjectID, request.InvestigationId.String(), request.FindingId.String()); err != nil {
		return nil, storeError(err)
	}
	return findings.DetachFinding204Response{}, nil
}

func convertFinding(item model.Finding) (findings.Finding, error) {
	var normalized, provenance any
	var contextErrors []model.GatewayContextError
	if err := json.Unmarshal(item.Normalized, &normalized); err != nil {
		return findings.Finding{}, err
	}
	if err := json.Unmarshal(item.Provenance, &provenance); err != nil {
		return findings.Finding{}, err
	}
	if err := json.Unmarshal(item.ContextErrors, &contextErrors); err != nil {
		return findings.Finding{}, err
	}
	payload := map[string]any{
		"id": item.ID, "ref": item.Ref, "kind": item.Kind, "title": item.Title,
		"description": item.Description, "severity": item.Severity, "occurred_at": item.OccurredAt,
		"status": item.Status, "source_ref": item.SourceRef, "fetched_at": item.FetchedAt,
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
		return findings.Finding{}, err
	}
	var out findings.Finding
	if err := json.Unmarshal(raw, &out); err != nil {
		return findings.Finding{}, err
	}
	return out, nil
}
