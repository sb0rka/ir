package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/somprompt"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/socctx"
	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/contract/som"
)

const somAccessTokenSecretName = "DEMO_SOM_ACCESS_TOKEN"

type somTokenCache struct {
	mu        sync.RWMutex
	projectID string
	token     string
}

func (c *somTokenCache) get(projectID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.projectID != projectID || c.token == "" {
		return "", false
	}
	return c.token, true
}

func (c *somTokenCache) replace(projectID, token string) {
	c.mu.Lock()
	c.projectID = projectID
	c.token = token
	c.mu.Unlock()
}

// ListSomWorkspaces SOM workspaces of the caller
// (GET /som/workspaces)
func (s *Server) ListSomWorkspaces(ctx context.Context, request som.ListSomWorkspacesRequestObject) (som.ListSomWorkspacesResponseObject, error) {
	// This endpoint is also the explicit activation probe used after the
	// dashboard creates a new secret version.
	var workspaces []somclient.Workspace
	err := s.withSOMBearer(ctx, true, func(bearer string) error {
		var callErr error
		workspaces, callErr = s.som.ListWorkspaces(ctx, bearer)
		return callErr
	})
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
	var boards []somclient.Board
	err := s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		boards, callErr = s.som.ListBoards(ctx, bearer, request.WorkspaceId.String())
		return callErr
	})
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
	var issues []somclient.Issue
	var total int
	err := s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		issues, total, callErr = s.som.ListIssues(ctx, bearer, request.BoardId.String())
		return callErr
	})
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

	if err := s.som.RunConfigured(); err != nil {
		return nil, somNotConfigured(err)
	}
	scope, err := s.scope(ctx)
	if err != nil {
		return nil, err
	}
	investigationID := request.Body.InvestigationId.String()
	if _, err := s.db.GetInvestigation(ctx, scope.ProjectID, investigationID); err != nil {
		return nil, storeError(err)
	}
	issueID := request.IssueId.String()
	var issue somclient.Issue
	err = s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		issue, callErr = s.som.GetIssue(ctx, bearer, issueID)
		return callErr
	})
	if err != nil {
		return nil, somError(err)
	}
	boardID, err := parseSomUUID("issue board id", issue.BoardID)
	if err != nil {
		return nil, err
	}

	description := ""
	if issue.Description != nil {
		description = *issue.Description
	}
	prompt := somprompt.Build(issue.Title, description, somprompt.Context{
		ProjectID:       scope.ProjectID,
		InvestigationID: investigationID,
		SomIssueID:      issue.ID,
	})
	name := runName(issue)
	exec := somclient.ResolveExecutorConfig(request.Body.Variant, request.Body.ModelId)

	var sessionID string
	err = s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		sessionID, callErr = s.som.CreateRelaySession(ctx, bearer)
		return callErr
	})
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
	var somEnvironmentID string
	err = s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		somEnvironmentID, callErr = s.som.LinkEnvironment(
			ctx, bearer, issue.BoardID, issueID, localEnvironmentID, name)
		return callErr
	})
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

// somBearer returns the project-scoped SOM token. The caller's verified JWT is
// used only to reveal the secret from Sb0rka API and is never cached.
func (s *Server) somBearer(ctx context.Context, forceReload bool) (string, error) {
	if !s.som.Configured() {
		return "", somNotConfigured(errors.New("SOM_API_BASE_URL is not set"))
	}
	scope, err := s.scope(ctx)
	if err != nil {
		return "", err
	}
	if !forceReload {
		if token, ok := s.somAuth.get(scope.ProjectID); ok {
			return token, nil
		}
	}
	if s.secrets == nil {
		return "", somNotConfigured(errors.New("SB0RKA_API_BASE_URL is not set"))
	}
	platformBearer, ok := socctx.BearerFromContext(ctx)
	if !ok {
		return "", somNotConfigured(errors.New("platform access token is required to read Secrets"))
	}
	snapshot, err := s.secrets.ResolveSnapshot(ctx, platformBearer, scope.ProjectID, somAccessTokenSecretName)
	if err != nil {
		return "", somSecretError(err)
	}
	token, ok := snapshot.Value(somAccessTokenSecretName)
	token = strings.TrimSpace(token)
	if !ok || token == "" {
		return "", somNotConfigured(errors.New("DEMO_SOM_ACCESS_TOKEN is empty"))
	}
	s.somAuth.replace(scope.ProjectID, token)
	return token, nil
}

// withSOMBearer retries only authentication denials. A rejected request has
// not executed the upstream operation; timeouts and 5xx responses are not
// retried because mutating SOM calls could otherwise be duplicated.
func (s *Server) withSOMBearer(ctx context.Context, forceFirstReload bool, call func(string) error) error {
	bearer, err := s.somBearer(ctx, forceFirstReload)
	if err != nil {
		return err
	}
	err = call(bearer)
	if !isSOMAuthError(err) {
		return err
	}
	bearer, err = s.somBearer(ctx, true)
	if err != nil {
		return err
	}
	return call(bearer)
}

func isSOMAuthError(err error) bool {
	var upstream *somclient.UpstreamError
	return errors.As(err, &upstream) &&
		(upstream.Status == http.StatusUnauthorized || upstream.Status == http.StatusForbidden)
}

func somSecretError(err error) error {
	switch {
	case errors.Is(err, common.ErrForbidden):
		return httperr.ErrForbidden
	case errors.Is(err, common.ErrSecretNotFound):
		return somNotConfigured(errors.New("DEMO_SOM_ACCESS_TOKEN is not configured"))
	default:
		return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable,
			"cannot load SOM access token from Sb0rka Secrets")
	}
}

func somNotConfigured(err error) error {
	return httperr.New(http.StatusBadGateway, httperr.CodeSourceUnavailable,
		"SOM integration is not configured: "+err.Error())
}

// SOM authentication failures describe the configured integration credential,
// not the dashboard JWT. They must not look like an expired user session.
func somError(err error) error {
	var upstream *somclient.UpstreamError
	if errors.As(err, &upstream) {
		switch upstream.Status {
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
	if err := s.som.RunConfigured(); err != nil {
		return nil, somNotConfigured(err)
	}

	var sessionID string
	err := s.withSOMBearer(ctx, false, func(bearer string) error {
		var callErr error
		sessionID, callErr = s.som.CreateRelaySession(ctx, bearer)
		return callErr
	})
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
