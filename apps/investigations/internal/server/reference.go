package server

import (
	"context"

	"github.com/sb0rka/ir/packages/contract/reference"
)

func (s *Server) GetReference(ctx context.Context, request reference.GetReferenceRequestObject) (reference.GetReferenceResponseObject, error) {
	if _, err := s.scope(ctx); err != nil {
		return nil, err
	}
	data, err := s.db.Reference(ctx)
	if err != nil {
		return nil, err
	}
	out := reference.Reference{EntityTypes: make([]reference.EntityType, 0, len(data.EntityTypes)), RelationTypes: make([]reference.RelationType, 0, len(data.RelationTypes)), Sources: make([]reference.Source, 0, len(data.Sources))}
	for _, item := range data.EntityTypes {
		out.EntityTypes = append(out.EntityTypes, reference.EntityType{Code: item.Code, Title: item.Title, Category: reference.EntityCategory(item.Category)})
	}
	for _, item := range data.RelationTypes {
		out.RelationTypes = append(out.RelationTypes, reference.RelationType{Code: item.Code, Title: item.Title, SourceKind: reference.NodeType(item.SourceKind), TargetKind: reference.NodeType(item.TargetKind), Directed: item.Directed})
	}
	for _, item := range data.Sources {
		out.Sources = append(out.Sources, reference.Source{Code: item.Code, Title: item.Title, Kind: reference.SourceKind(item.Kind)})
	}
	return reference.GetReference200JSONResponse(out), nil
}
