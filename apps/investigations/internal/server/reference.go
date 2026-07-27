package server

import (
	"context"

	"github.com/sb0rka/ir/packages/contract/reference"
)

// Типы сущностей
// GET /entity-types
func (s *Server) ListEntityTypes(
	ctx context.Context, _ reference.ListEntityTypesRequestObject,
) (reference.ListEntityTypesResponseObject, error) {
	items, err := s.db.ListEntityTypes(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]reference.EntityType, 0, len(items))
	for _, item := range items {
		out = append(out, reference.EntityType{Code: item.Code, Title: item.Title})
	}
	return reference.ListEntityTypes200JSONResponse{Items: out}, nil
}

// Типы связей
// GET /relation-types
func (s *Server) ListRelationTypes(
	ctx context.Context, _ reference.ListRelationTypesRequestObject,
) (reference.ListRelationTypesResponseObject, error) {
	items, err := s.db.ListRelationTypes(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]reference.RelationType, 0, len(items))
	for _, item := range items {
		out = append(out, reference.RelationType{
			Code:       item.Code,
			Title:      item.Title,
			SourceKind: reference.NodeType(item.SourceKind),
			TargetKind: reference.NodeType(item.TargetKind),
			Directed:   item.Directed,
		})
	}
	return reference.ListRelationTypes200JSONResponse{Items: out}, nil
}
