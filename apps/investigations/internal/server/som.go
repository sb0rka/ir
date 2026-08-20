package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/somprompt"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/contract/som"
)

// ListSomWorkspaces SOM workspaces of the caller
// (GET /som/workspaces)
func (s *Server) ListSomWorkspaces(ctx context.Context, request som.ListSomWorkspacesRequestObject) (som.ListSomWorkspacesResponseObject, error) {
	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}

	workspaces, err := s.som.ListWorkspaces(ctx, bearer)
	if err != nil {
		return nil, somError(err)
	}

	out := make([]som.SomWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		id, err := parseSomUUID("workspace id", workspace.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, som.SomWorkspace{
			Id:          id,
			Name:        workspace.Name,
			Slug:        workspace.Slug,
			IsPersonal:  workspace.IsPersonal,
			IssuePrefix: workspace.IssuePrefix,
			UserRole:    workspace.UserRole,
		})
	}
	return som.ListSomWorkspaces200JSONResponse(out), nil
}

// ListSomBoards Boards of a SOM workspace
// (GET /som/workspaces/{workspace_id}/boards)
func (s *Server) ListSomBoards(ctx context.Context, request som.ListSomBoardsRequestObject) (som.ListSomBoardsResponseObject, error) {
	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}

	boards, err := s.som.ListBoards(ctx, bearer, request.WorkspaceId.String())
	if err != nil {
		return nil, somError(err)
	}

	out := make([]som.SomBoard, 0, len(boards))
	for _, board := range boards {
		id, err := parseSomUUID("board id", board.ID)
		if err != nil {
			return nil, err
		}
		workspaceID, err := parseSomUUID("board workspace id", board.WorkspaceID)
		if err != nil {
			return nil, err
		}
		out = append(out, som.SomBoard{
			Id:          id,
			WorkspaceId: workspaceID,
			Name:        board.Name,
		})
	}
	return som.ListSomBoards200JSONResponse(out), nil
}

// ListSomIssues Issues of a SOM board
// (GET /som/boards/{board_id}/issues)
func (s *Server) ListSomIssues(ctx context.Context, request som.ListSomIssuesRequestObject) (som.ListSomIssuesResponseObject, error) {
	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}

	issues, total, err := s.som.ListIssues(ctx, bearer, request.BoardId.String())
	if err != nil {
		return nil, somError(err)
	}

	out := som.SomIssueList{Issues: make([]som.SomIssue, 0, len(issues)), TotalCount: total}
	for _, issue := range issues {
		converted, err := convertSomIssue(issue)
		if err != nil {
			return nil, err
		}
		out.Issues = append(out.Issues, converted)
	}
	return som.ListSomIssues200JSONResponse(out), nil
}

// RunSomIssue Run an agent on a SOM issue
// (POST /som/issues/{issue_id}/run)
func (s *Server) RunSomIssue(ctx context.Context, request som.RunSomIssueRequestObject) (som.RunSomIssueResponseObject, error) {
	if request.Body == nil {
		return nil, httperr.BadRequest("request body is required")
	}
	if request.Body.InvestigationId == uuid.Nil {
		return nil, httperr.New(http.StatusUnprocessableEntity, httperr.CodeValidation,
			"investigation_id must be a non-zero UUID")
	}

	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.som.RunConfigured(); err != nil {
		return nil, somNotConfigured(err)
	}
	if s.prompt.GatewayBaseURL == "" {
		return nil, somNotConfigured(errors.New("GATEWAY_PUBLIC_BASE_URL is not set"))
	}
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}

	issueID := request.IssueId.String()
	issue, err := s.som.GetIssue(ctx, bearer, issueID)
	if err != nil {
		return nil, somError(err)
	}
	boardID, err := parseSomUUID("issue board id", issue.BoardID)
	if err != nil {
		return nil, err
	}

	investigationID := request.Body.InvestigationId.String()
	description := ""
	if issue.Description != nil {
		description = *issue.Description
	}
	prompt := somprompt.Build(issue.Title, description, somprompt.Context{
		IRBaseURL:       s.prompt.IRBaseURL,
		GatewayBaseURL:  s.prompt.GatewayBaseURL,
		ProjectID:       scope.ProjectID,
		InvestigationID: investigationID,
		SomIssueID:      issue.ID,
	})
	name := runName(issue)
	exec := somclient.ResolveExecutorConfig(request.Body.Variant, request.Body.ModelId)

	sessionID, err := s.som.CreateRelaySession(ctx, bearer)
	if err != nil {
		return nil, somError(err)
	}
	repoID, err := s.som.EnsureRepo(ctx, sessionID)
	if err != nil {
		return nil, somError(err)
	}
	localEnvironmentID, err := s.som.StartEnvironment(ctx, sessionID, repoID, name, prompt, exec)
	if err != nil {
		return nil, somError(err)
	}
	somEnvironmentID, err := s.som.LinkEnvironment(ctx, bearer, issue.BoardID, issueID, localEnvironmentID, name)
	if err != nil {
		return nil, somError(err)
	}

	localEnvUUID, err := parseSomUUID("daemon environment id", localEnvironmentID)
	if err != nil {
		return nil, err
	}
	somEnvUUID, err := parseSomUUID("som environment id", somEnvironmentID)
	if err != nil {
		return nil, err
	}
	repoUUID, err := parseSomUUID("daemon repo id", repoID)
	if err != nil {
		return nil, err
	}

	s.log.Info("som_issue_run_started",
		"issue_id", issueID,
		"board_id", issue.BoardID,
		"local_environment_id", localEnvironmentID,
		"som_environment_id", somEnvironmentID,
		"model_id", exec.ModelID,
		"variant", exec.Variant)

	return som.RunSomIssue201JSONResponse(som.SomIssueRunResult{
		IssueId:            request.IssueId,
		BoardId:            boardID,
		LocalEnvironmentId: localEnvUUID,
		SomEnvironmentId:   somEnvUUID,
		RepoId:             &repoUUID,
	}), nil
}

func runName(issue somclient.Issue) string {
	name := strings.TrimSpace(issue.SimpleID + " " + issue.Title)
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func convertSomIssue(issue somclient.Issue) (som.SomIssue, error) {
	id, err := parseSomUUID("issue id", issue.ID)
	if err != nil {
		return som.SomIssue{}, err
	}
	boardID, err := parseSomUUID("issue board id", issue.BoardID)
	if err != nil {
		return som.SomIssue{}, err
	}
	out := som.SomIssue{
		Id:          id,
		BoardId:     boardID,
		IssueNumber: issue.IssueNumber,
		SimpleId:    issue.SimpleID,
		Title:       issue.Title,
		Description: issue.Description,
		Priority:    issue.Priority,
	}
	if issue.ParentIssueID != nil {
		parentID, err := parseSomUUID("issue parent id", *issue.ParentIssueID)
		if err != nil {
			return som.SomIssue{}, err
		}
		out.ParentIssueId = &parentID
	}
	return out, nil
}

// somBearer достаёт pass-through токен вызывающего. Без токена в SOM идти
// не с чем — 401 честнее, чем 502 от самого SOM.
func (s *Server) somBearer(ctx context.Context) (string, error) {
	if !s.som.Configured() {
		return "", somNotConfigured(errors.New("SOM_API_BASE_URL is not set"))
	}
	bearer, ok := socctx.BearerFromContext(ctx)
	if !ok {
		return "", httperr.New(http.StatusUnauthorized, httperr.CodeUnauthorized,
			"SOM access token is required: pass it as Authorization: Bearer")
	}
	return bearer, nil
}

func somNotConfigured(err error) error {
	return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable,
		"SOM integration is not configured: "+err.Error())
}

// somError переводит ошибки клиента в конверт httperr. 401/403/404 от SOM
// пробрасываются как есть — это ответ про токен и права вызывающего, а не
// про доступность SOM.
func somError(err error) error {
	var upstream *somclient.UpstreamError
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case http.StatusUnauthorized:
			return httperr.New(http.StatusUnauthorized, httperr.CodeUnauthorized,
				"SOM rejected the token: "+upstream.Body)
		case http.StatusForbidden:
			return httperr.ErrForbidden
		case http.StatusNotFound:
			return httperr.ErrNotFound
		}
		return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable, upstream.Error())
	}
	return err
}

func parseSomUUID(field, value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable,
			"SOM returned a malformed "+field+": "+value)
	}
	return parsed, nil
}

// GetSomEnvironment Status of a SOM agent environment
// (GET /som/environments/{local_environment_id})
func (s *Server) GetSomEnvironment(ctx context.Context, request som.GetSomEnvironmentRequestObject) (som.GetSomEnvironmentResponseObject, error) {
	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.som.RunConfigured(); err != nil {
		return nil, somNotConfigured(err)
	}

	sessionID, err := s.som.CreateRelaySession(ctx, bearer)
	if err != nil {
		return nil, somError(err)
	}
	env, err := s.som.GetEnvironment(ctx, sessionID, request.LocalEnvironmentId.String())
	if err != nil {
		return nil, somError(err)
	}

	status := mapDaemonEnvironmentStatus(env.IsRunning, env.IsErrored)
	isRunning := env.IsRunning
	isErrored := env.IsErrored
	return som.GetSomEnvironment200JSONResponse(som.SomEnvironmentStatus{
		LocalEnvironmentId: request.LocalEnvironmentId,
		Status:             status,
		IsRunning:          &isRunning,
		IsErrored:          &isErrored,
	}), nil
}

// mapDaemonEnvironmentStatus: errored wins over running; idle+ok → completed.
func mapDaemonEnvironmentStatus(isRunning, isErrored bool) som.SomEnvironmentStatusStatus {
	if isErrored {
		return som.Failed
	}
	if isRunning {
		return som.Running
	}
	return som.Completed
}
