package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/entities"
)

func (s *Server) ListEntities(ctx context.Context, request entities.ListEntitiesRequestObject) (entities.ListEntitiesResponseObject, error) {
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
	filter := model.EntityFilter{TypeCode: request.Params.TypeCode, Cursor: cursor, Limit: limit + 1}
	if request.Params.Q != nil {
		v := strings.TrimSpace(*request.Params.Q)
		if v != "" {
			filter.Q = &v
		}
	}
	items, err := s.db.InvestigationEntities(ctx, scope.ProjectID, request.InvestigationId.String(), filter)
	if err != nil {
		return nil, storeError(err)
	}
	page := entities.EntityPage{Entities: make([]entities.Entity, 0, min(limit, len(items)))}
	if len(items) > limit {
		last := items[limit-1]
		next := encodeCursor(last.AddedAt, last.ID)
		page.NextCursor = &next
		items = items[:limit]
	}
	for _, item := range items {
		converted, err := convertEntity(item)
		if err != nil {
			return nil, err
		}
		page.Entities = append(page.Entities, converted)
	}
	return entities.ListEntities200JSONResponse(page), nil
}

func (s *Server) CreateEntity(ctx context.Context, request entities.CreateEntityRequestObject) (entities.CreateEntityResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	typeCode := strings.TrimSpace(request.Body.TypeCode)
	key := strings.TrimSpace(request.Body.CanonicalKey)
	if typeCode == "" || key == "" {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation, "type_code and canonical_key are required")
	}
	metadata := []byte("{}")
	if request.Body.Metadata != nil {
		metadata, _ = json.Marshal(*request.Body.Metadata)
	}
	item, err := s.db.CreateEntity(ctx, model.EntityNew{ProjectID: scope.ProjectID, InvestigationID: request.InvestigationId.String(), TypeCode: typeCode, CanonicalKey: key, DisplayName: request.Body.DisplayName, Metadata: metadata})
	if err != nil {
		return nil, storeError(err)
	}
	out, err := convertEntity(item)
	if err != nil {
		return nil, err
	}
	return entities.CreateEntity201JSONResponse(out), nil
}

func (s *Server) GetEntityCard(ctx context.Context, request entities.GetEntityCardRequestObject) (entities.GetEntityCardResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.db.GetEntityCard(ctx, scope.ProjectID, request.EntityId.String())
	if err != nil {
		return nil, storeError(err)
	}
	entity, err := convertEntity(item.Entity)
	if err != nil {
		return nil, err
	}
	out := entities.EntityCard{Entity: entity, EventsCount: item.EventsCount, Occurrences: make([]struct {
		EventsCount     int                `json:"events_count"`
		InvestigationId openapi_types.UUID `json:"investigation_id"`
		Title           string             `json:"title"`
	}, 0, len(item.Occurrences))}
	for _, occ := range item.Occurrences {
		id, err := dbUUID(occ.InvestigationID)
		if err != nil {
			return nil, err
		}
		out.Occurrences = append(out.Occurrences, struct {
			EventsCount     int                `json:"events_count"`
			InvestigationId openapi_types.UUID `json:"investigation_id"`
			Title           string             `json:"title"`
		}{occ.EventsCount, id, occ.Title})
	}
	neighbors := make([]struct {
		DisplayName  *string            `json:"display_name,omitempty"`
		EntityId     openapi_types.UUID `json:"entity_id"`
		RelationCode string             `json:"relation_code"`
	}, 0, len(item.Neighbors))
	for _, neighbor := range item.Neighbors {
		id, err := dbUUID(neighbor.EntityID)
		if err != nil {
			return nil, err
		}
		neighbors = append(neighbors, struct {
			DisplayName  *string            `json:"display_name,omitempty"`
			EntityId     openapi_types.UUID `json:"entity_id"`
			RelationCode string             `json:"relation_code"`
		}{neighbor.DisplayName, id, neighbor.RelationCode})
	}
	out.Neighbors = &neighbors
	return entities.GetEntityCard200JSONResponse(out), nil
}

func convertEntity(item model.Entity) (entities.Entity, error) {
	id, err := dbUUID(item.ID)
	if err != nil {
		return entities.Entity{}, err
	}
	out := entities.Entity{Id: id, TypeCode: item.TypeCode, CanonicalKey: item.CanonicalKey, DisplayName: item.DisplayName, FirstSeen: item.FirstSeen, LastSeen: item.LastSeen, Sources: []entities.EntitySource{}}
	for _, source := range item.Sources {
		out.Sources = append(out.Sources, entities.EntitySource{SourceCode: source.SourceCode, SourceEntityId: source.SourceEntityID, SourceRef: source.SourceRef, FetchedAt: source.FetchedAt})
	}
	if item.AddedVia != nil {
		origin := entities.EntityOrigin(*item.AddedVia)
		out.AddedVia = &origin
	}
	var metadata map[string]any
	if len(item.Metadata) > 0 {
		if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
			return entities.Entity{}, err
		}
		out.Metadata = &metadata
	}
	return out, nil
}

func (s *Server) DetachEntity(ctx context.Context, request entities.DetachEntityRequestObject) (entities.DetachEntityResponseObject, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DetachEntity(ctx, scope.ProjectID, request.InvestigationId.String(), request.EntityId.String()); err != nil {
		return nil, storeError(err)
	}
	return entities.DetachEntity204Response{}, nil
}
func (s *Server) UpdateEntity(context.Context, entities.UpdateEntityRequestObject) (entities.UpdateEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
func (s *Server) DeleteEntity(context.Context, entities.DeleteEntityRequestObject) (entities.DeleteEntityResponseObject, error) {
	return nil, httperr.ErrNotImplemented
}
