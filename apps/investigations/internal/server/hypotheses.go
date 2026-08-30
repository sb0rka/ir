package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/hypotheses"
)

// ListHypotheses List hypotheses of an investigation
// (GET /investigations/{investigation_id}/hypotheses)
func (s *Server) ListHypotheses(ctx context.Context, request hypotheses.ListHypothesesRequestObject) (hypotheses.ListHypothesesResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, validationError("limit must be between 1 and 200")
	}
	cursor, err := decodeCursor(request.Params.Cursor)
	if err != nil {
		return nil, err
	}
	filter := model.HypothesisFilter{Cursor: cursor, Limit: limit + 1}
	if request.Params.Status != nil {
		if !request.Params.Status.Valid() {
			return nil, validationError("invalid hypothesis status")
		}
		value := string(*request.Params.Status)
		filter.Status = &value
	}
	items, err := s.db.ListHypotheses(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	page := hypotheses.HypothesisPage{Hypotheses: make([]hypotheses.Hypothesis, 0, min(len(items), limit))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertHypothesis(item)
		if err != nil {
			return nil, err
		}
		page.Hypotheses = append(page.Hypotheses, converted)
	}
	return hypotheses.ListHypotheses200JSONResponse(page), nil
}

// CreateHypothesis Propose a hypothesis
// (POST /investigations/{investigation_id}/hypotheses)
func (s *Server) CreateHypothesis(ctx context.Context, request hypotheses.CreateHypothesisRequestObject) (hypotheses.CreateHypothesisResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	statement := strings.TrimSpace(request.Body.Statement)
	if statement == "" || utf8.RuneCountInString(statement) > 255 {
		return nil, validationError("statement must be 1-255 characters")
	}
	created, err := s.db.CreateHypothesis(ctx, model.HypothesisNew{
		ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(),
		Statement: statement, Description: request.Body.Description,
	})
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	out, err := convertHypothesis(created)
	if err != nil {
		return nil, err
	}
	return hypotheses.CreateHypothesis201JSONResponse(out), nil
}

// DeleteHypothesis Delete a hypothesis
// (DELETE /investigations/{investigation_id}/hypotheses/{hypothesis_id})
func (s *Server) DeleteHypothesis(ctx context.Context, request hypotheses.DeleteHypothesisRequestObject) (hypotheses.DeleteHypothesisResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteHypothesis(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String()); err != nil {
		return nil, hypothesisStoreError(err)
	}
	return hypotheses.DeleteHypothesis204Response{}, nil
}

// GetHypothesis One hypothesis
// (GET /investigations/{investigation_id}/hypotheses/{hypothesis_id})
func (s *Server) GetHypothesis(ctx context.Context, request hypotheses.GetHypothesisRequestObject) (hypotheses.GetHypothesisResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetHypothesis(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String())
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	out, err := convertHypothesis(item)
	if err != nil {
		return nil, err
	}
	return hypotheses.GetHypothesis200JSONResponse(out), nil
}

// UpdateHypothesis Update a hypothesis
// (PATCH /investigations/{investigation_id}/hypotheses/{hypothesis_id})
func (s *Server) UpdateHypothesis(ctx context.Context, request hypotheses.UpdateHypothesisRequestObject) (hypotheses.UpdateHypothesisResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if request.Body.Version < 1 {
		return nil, validationError("version must be positive")
	}
	patch := model.HypothesisPatch{
		ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(),
		HypothesisID: request.HypothesisId.String(), Version: request.Body.Version,
		Description: request.Body.Description, HasDescription: request.Body.Description != nil,
		Reason: request.Body.Reason,
	}
	if request.Body.Statement != nil {
		value := strings.TrimSpace(*request.Body.Statement)
		if value == "" || utf8.RuneCountInString(value) > 255 {
			return nil, validationError("statement must be 1-255 characters")
		}
		patch.Statement = &value
	}
	if request.Body.Status != nil {
		value := string(*request.Body.Status)
		patch.Status = &value
	}
	updated, err := s.db.UpdateHypothesis(ctx, patch)
	if err != nil {
		return nil, hypothesisStoreError(err)
	}
	out, err := convertHypothesis(updated)
	if err != nil {
		return nil, err
	}
	return hypotheses.UpdateHypothesis200JSONResponse(out), nil
}

// DeleteHypothesisEdge Remove an edge from a hypothesis
// (DELETE /investigations/{investigation_id}/hypotheses/{hypothesis_id}/edges/{edge_id})
func (s *Server) DeleteHypothesisEdge(ctx context.Context, request hypotheses.DeleteHypothesisEdgeRequestObject) (hypotheses.DeleteHypothesisEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteHypothesisEdge(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String(), request.EdgeId.String()); err != nil {
		return nil, hypothesisStoreError(err)
	}
	return hypotheses.DeleteHypothesisEdge204Response{}, nil
}

// AddHypothesisEdge Add an existing graph edge to a hypothesis
// (PUT /investigations/{investigation_id}/hypotheses/{hypothesis_id}/edges/{edge_id})
func (s *Server) AddHypothesisEdge(ctx context.Context, request hypotheses.AddHypothesisEdgeRequestObject) (hypotheses.AddHypothesisEdgeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.AddHypothesisEdge(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String(), request.EdgeId.String()); err != nil {
		return nil, hypothesisStoreError(err)
	}
	return hypotheses.AddHypothesisEdge204Response{}, nil
}

// DeleteHypothesisNode Remove a node from a hypothesis
// (DELETE /investigations/{investigation_id}/hypotheses/{hypothesis_id}/nodes/{node_id})
func (s *Server) DeleteHypothesisNode(ctx context.Context, request hypotheses.DeleteHypothesisNodeRequestObject) (hypotheses.DeleteHypothesisNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteHypothesisNode(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String(), request.NodeId.String()); err != nil {
		return nil, hypothesisStoreError(err)
	}
	return hypotheses.DeleteHypothesisNode204Response{}, nil
}

// AddHypothesisNode Add an existing graph node to a hypothesis
// (PUT /investigations/{investigation_id}/hypotheses/{hypothesis_id}/nodes/{node_id})
func (s *Server) AddHypothesisNode(ctx context.Context, request hypotheses.AddHypothesisNodeRequestObject) (hypotheses.AddHypothesisNodeResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.AddHypothesisNode(ctx, scope.ProjectID, request.InvestigationId.String(), request.HypothesisId.String(), request.NodeId.String()); err != nil {
		return nil, hypothesisStoreError(err)
	}
	return hypotheses.AddHypothesisNode204Response{}, nil
}

func convertHypothesis(item model.Hypothesis) (hypotheses.Hypothesis, error) {
	id, err := dbUUID(item.ID)
	if err != nil {
		return hypotheses.Hypothesis{}, err
	}
	investigationID, err := dbUUID(item.InvestigationID)
	if err != nil {
		return hypotheses.Hypothesis{}, err
	}
	return hypotheses.Hypothesis{
		Id: id, ProjectId: item.ProjectID, InvestigationId: investigationID,
		Statement: item.Statement, Description: item.Description,
		Status: hypotheses.HypothesisStatus(item.Status), Reason: item.Reason,
		Origin: hypotheses.Origin(item.Origin), Version: item.Version,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ResolvedAt: item.ResolvedAt,
	}, nil
}

func hypothesisStoreError(err error) error {
	var conflict *store.ConflictError
	if errors.As(err, &conflict) {
		return httperr.New(http.StatusConflict, httperr.CodeConflict,
			"operation conflicts with the current hypothesis state").WithDetails(map[string]any{"conflicts": conflict.IDs})
	}
	return storeError(err)
}
