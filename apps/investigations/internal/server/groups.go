package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
	"github.com/sb0rka/ir/apps/investigations/internal/store"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/groups"
	"github.com/sb0rka/ir/packages/contract/investigations"
	coreauthctx "github.com/sb0rka/sb0rka/packages/core/transport/authctx"
)

// Group DTOs share JSON shapes with the transport-independent model.
// Conversion stays at this boundary and never swallows serialization errors.
func groupDTO[T any](value any) (T, error) {
	var out T
	raw, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}
func groupStoreError(err error) error {
	if errors.Is(err, store.ErrConflict) {
		return httperr.New(http.StatusConflict, httperr.CodeConflict, "group version, membership, or operation conflicts with the current state")
	}
	return storeError(err)
}
func projectionStatuses[T ~string](input *[]T) []string {
	out := []string{}
	if input != nil {
		for _, v := range *input {
			out = append(out, string(v))
		}
	}
	return out
}
func (s *Server) groupProjection(ctx context.Context, investigationID uuid.UUID, hypothesisID *uuid.UUID, include bool, statuses []string, minConfidence *float32) (groups.GraphProjection, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return groups.GraphProjection{}, err
	}
	r := model.ProjectionRequest{ProjectID: scope.ProjectID, InvestigationID: investigationID.String(), Filter: model.EdgeFilter{IncludeSubtree: include, Statuses: statuses, MinConfidence: minConfidence}}
	if hypothesisID != nil {
		id := hypothesisID.String()
		r.HypothesisID = &id
	}
	value, err := s.db.GraphProjection(ctx, r)
	if err != nil {
		return groups.GraphProjection{}, groupStoreError(err)
	}
	return groupDTO[groups.GraphProjection](value)
}
func (s *Server) GetGraphProjection(ctx context.Context, r groups.GetGraphProjectionRequestObject) (groups.GetGraphProjectionResponseObject, error) {
	out, err := s.groupProjection(ctx, r.InvestigationId, nil, r.Params.IncludeSubtree != nil && *r.Params.IncludeSubtree, projectionStatuses(r.Params.Statuses), r.Params.MinConfidence)
	if err != nil {
		return nil, err
	}
	return groups.GetGraphProjection200JSONResponse(out), nil
}
func (s *Server) GetHypothesisGraphProjection(ctx context.Context, r groups.GetHypothesisGraphProjectionRequestObject) (groups.GetHypothesisGraphProjectionResponseObject, error) {
	out, err := s.groupProjection(ctx, r.InvestigationId, &r.HypothesisId, false, projectionStatuses(r.Params.Statuses), r.Params.MinConfidence)
	if err != nil {
		return nil, err
	}
	return groups.GetHypothesisGraphProjection200JSONResponse(out), nil
}
func (s *Server) groupDetail(ctx context.Context, root, id uuid.UUID, family string) (groups.Group, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return groups.Group{}, err
	}
	value, err := s.db.GetGroup(ctx, model.GroupScope{ProjectID: scope.ProjectID, RootID: root.String()}, family, id.String())
	if err != nil {
		return groups.Group{}, groupStoreError(err)
	}
	return groupDTO[groups.Group](value)
}
func (s *Server) groupHistory(ctx context.Context, root, id uuid.UUID, family string, cursor *string, limit *int) (groups.GroupHistory, error) {
	scope, err := s.scope(ctx)
	if err != nil {
		return groups.GroupHistory{}, err
	}
	size := 50
	if limit != nil {
		size = *limit
	}
	value, err := s.db.GroupHistory(ctx, model.GroupScope{ProjectID: scope.ProjectID, RootID: root.String()}, family, id.String(), cursor, size)
	if err != nil {
		return groups.GroupHistory{}, groupStoreError(err)
	}
	return groupDTO[groups.GroupHistory](value)
}
func (s *Server) groupMutation(ctx context.Context, root, id uuid.UUID, family, action string, body any) (groups.GroupMutationResult, error) {
	var out groups.GroupMutationResult
	scope, err := s.scope(ctx)
	if err != nil {
		return out, err
	}
	actor, ok := coreauthctx.SubjectIDFromContext(ctx)
	if !ok || actor == "" {
		return out, httperr.New(http.StatusUnauthorized, httperr.CodeUnauthorized, "authenticated actor is required")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	if string(raw) == "null" {
		return out, httperr.BadRequest("request body is required")
	}
	r := model.GroupMutation{GroupScope: model.GroupScope{ProjectID: scope.ProjectID, RootID: root.String()}, Family: family, GroupID: id.String(), Actor: actor}
	switch action {
	case "review":
		r.Review = &model.GroupReview{}
		err = json.Unmarshal(raw, r.Review)
	case "merge":
		r.Merge = &model.GroupMerge{}
		err = json.Unmarshal(raw, r.Merge)
	case "split":
		r.Split = &model.GroupSplit{}
		err = json.Unmarshal(raw, r.Split)
	default:
		return out, validationError("invalid group operation")
	}
	if err != nil {
		return out, err
	}
	value, err := s.db.MutateGroup(ctx, r)
	if err != nil {
		return out, groupStoreError(err)
	}
	out.Groups, err = groupDTO[[]groups.Group](value)
	return out, err
}
func importGroupProposals(body investigations.AgentResultBatch, input *model.ImportRequest) error {
	var err error
	if body.EntityGroupProposals != nil {
		input.EntityGroupProposals, err = groupDTO[[]model.GroupProposal](*body.EntityGroupProposals)
		if err != nil {
			return err
		}
	}
	if body.EventGroupProposals != nil {
		input.EventGroupProposals, err = groupDTO[[]model.GroupProposal](*body.EventGroupProposals)
	}
	return err
}

func (s *Server) GetEntityGroup(ctx context.Context, r groups.GetEntityGroupRequestObject) (groups.GetEntityGroupResponseObject, error) {
	out, err := s.groupDetail(ctx, r.RootId, r.GroupId, "entity")
	if err != nil {
		return nil, err
	}
	return groups.GetEntityGroup200JSONResponse(out), nil
}

func (s *Server) GetEntityGroupHistory(ctx context.Context, r groups.GetEntityGroupHistoryRequestObject) (groups.GetEntityGroupHistoryResponseObject, error) {
	out, err := s.groupHistory(ctx, r.RootId, r.GroupId, "entity", r.Params.Cursor, r.Params.Limit)
	if err != nil {
		return nil, err
	}
	return groups.GetEntityGroupHistory200JSONResponse(out), nil
}

func (s *Server) ReviewEntityGroup(ctx context.Context, r groups.ReviewEntityGroupRequestObject) (groups.ReviewEntityGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "entity", "review", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.ReviewEntityGroup200JSONResponse(out), nil
}

func (s *Server) MergeEntityGroup(ctx context.Context, r groups.MergeEntityGroupRequestObject) (groups.MergeEntityGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "entity", "merge", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.MergeEntityGroup200JSONResponse(out), nil
}

func (s *Server) SplitEntityGroup(ctx context.Context, r groups.SplitEntityGroupRequestObject) (groups.SplitEntityGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "entity", "split", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.SplitEntityGroup200JSONResponse(out), nil
}

func (s *Server) GetEventGroup(ctx context.Context, r groups.GetEventGroupRequestObject) (groups.GetEventGroupResponseObject, error) {
	out, err := s.groupDetail(ctx, r.RootId, r.GroupId, "event")
	if err != nil {
		return nil, err
	}
	return groups.GetEventGroup200JSONResponse(out), nil
}

func (s *Server) GetEventGroupHistory(ctx context.Context, r groups.GetEventGroupHistoryRequestObject) (groups.GetEventGroupHistoryResponseObject, error) {
	out, err := s.groupHistory(ctx, r.RootId, r.GroupId, "event", r.Params.Cursor, r.Params.Limit)
	if err != nil {
		return nil, err
	}
	return groups.GetEventGroupHistory200JSONResponse(out), nil
}

func (s *Server) ReviewEventGroup(ctx context.Context, r groups.ReviewEventGroupRequestObject) (groups.ReviewEventGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "event", "review", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.ReviewEventGroup200JSONResponse(out), nil
}

func (s *Server) MergeEventGroup(ctx context.Context, r groups.MergeEventGroupRequestObject) (groups.MergeEventGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "event", "merge", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.MergeEventGroup200JSONResponse(out), nil
}

func (s *Server) SplitEventGroup(ctx context.Context, r groups.SplitEventGroupRequestObject) (groups.SplitEventGroupResponseObject, error) {
	out, err := s.groupMutation(ctx, r.RootId, r.GroupId, "event", "split", r.Body)
	if err != nil {
		return nil, err
	}
	return groups.SplitEventGroup200JSONResponse(out), nil
}
