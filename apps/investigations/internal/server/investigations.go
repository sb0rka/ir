package server

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/investigations"
)

func (s *Server) ListInvestigations(
	ctx context.Context, request investigations.ListInvestigationsRequestObject,
) (investigations.ListInvestigationsResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Params.Cursor != nil {
		return nil, httperr.ErrNotImplemented
	}

	limit := 50
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if limit < 1 || limit > 200 {
		return nil, httperr.New(422, httperr.CodeValidation, "limit must be between 1 and 200")
	}

	filter := model.InvestigationFilter{Limit: limit, RootsOnly: request.Params.ParentId == nil}
	if request.Params.ParentId != nil {
		parentID := request.Params.ParentId.String()
		filter.ParentID = &parentID
		filter.RootsOnly = false
	}
	if request.Params.Status != nil {
		status := string(*request.Params.Status)
		filter.Status = &status
	}
	if request.Params.Severity != nil {
		severity := string(*request.Params.Severity)
		filter.Severity = &severity
	}
	if request.Params.Q != nil {
		q := strings.TrimSpace(*request.Params.Q)
		if q != "" {
			filter.Q = &q
		}
	}

	items, err := s.db.ListInvestigations(ctx, scope.ProjectID, filter)
	if err != nil {
		return nil, storeError(err)
	}

	page := investigations.InvestigationPage{Items: make([]investigations.Investigation, 0, len(items))}
	for _, item := range items {
		converted, err := convertInvestigation(item)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, converted)
	}
	return investigations.ListInvestigations200JSONResponse(page), nil
}

func (s *Server) CreateInvestigation(
	ctx context.Context, request investigations.CreateInvestigationRequestObject,
) (investigations.CreateInvestigationResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	title := strings.TrimSpace(request.Body.Title)
	if title == "" || len(title) > 255 {
		return nil, httperr.New(422, httperr.CodeValidation, "title must be 1-255 characters")
	}

	create := model.InvestigationNew{
		ProjectID:   scope.ProjectID,
		Title:       title,
		Description: request.Body.Description,
		Severity:    (*string)(request.Body.Severity),
	}
	if request.Body.ParentId != nil {
		parentID := request.Body.ParentId.String()
		create.ParentID = &parentID
	}
	// Схема хранит одну ссылку на SOM workspace — берётся первая из списка.
	if request.Body.SomWorkspaceIds != nil && len(*request.Body.SomWorkspaceIds) > 0 {
		workspaceID := (*request.Body.SomWorkspaceIds)[0].String()
		create.WorkspaceID = &workspaceID
	}

	created, err := s.db.CreateInvestigation(ctx, create)
	if err != nil {
		return nil, storeError(err)
	}
	converted, err := convertInvestigation(created)
	if err != nil {
		return nil, err
	}
	return investigations.CreateInvestigation201JSONResponse(converted), nil
}

// convertInvestigation собирает контрактный вид только что созданного
// расследования: счётчики нулевые, потому что вложить в него ещё ничего
// не успели.
func convertInvestigation(inv model.Investigation) (investigations.Investigation, error) {
	id, err := dbUUID(inv.ID)
	if err != nil {
		return investigations.Investigation{}, err
	}

	origin := investigations.Origin(inv.Origin)
	out := investigations.Investigation{
		Id:              id,
		ProjectId:       inv.ProjectID,
		Title:           inv.Title,
		Description:     inv.Description,
		Status:          investigations.InvestigationStatus(inv.Status),
		Severity:        (*investigations.Severity)(inv.Severity),
		Origin:          &origin,
		Version:         inv.Version,
		SomWorkspaceIds: []uuid.UUID{},
		CreatedAt:       inv.CreatedAt,
		UpdatedAt:       inv.UpdatedAt,
	}
	out.Counters.Children = inv.Counters.Children
	out.Counters.Events = inv.Counters.Events
	out.Counters.Entities = inv.Counters.Entities
	out.Counters.ProposedEdges = inv.Counters.ProposedEdges
	if inv.ParentID != nil {
		parentID, err := dbUUID(*inv.ParentID)
		if err != nil {
			return investigations.Investigation{}, err
		}
		out.ParentId = &parentID
	}
	if inv.WorkspaceID != nil {
		workspaceID, err := dbUUID(*inv.WorkspaceID)
		if err != nil {
			return investigations.Investigation{}, err
		}
		out.SomWorkspaceIds = append(out.SomWorkspaceIds, workspaceID)
	}
	return out, nil
}

func (s *Server) GetInvestigation(
	_ context.Context, _ investigations.GetInvestigationRequestObject,
) (investigations.GetInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) UpdateInvestigation(
	_ context.Context, _ investigations.UpdateInvestigationRequestObject,
) (investigations.UpdateInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) GetInvestigationTree(
	_ context.Context, _ investigations.GetInvestigationTreeRequestObject,
) (investigations.GetInvestigationTreeResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}

func (s *Server) DeleteInvestigation(ctx context.Context, request investigations.DeleteInvestigationRequestObject) (investigations.DeleteInvestigationResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
