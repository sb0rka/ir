package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
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
	bearer, err := s.somBearer(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.som.RunConfigured(); err != nil {
		return nil, somNotConfigured(err)
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

	var investigationID string
	if request.Body != nil && request.Body.InvestigationId != nil {
		investigationID = request.Body.InvestigationId.String()
	}
	prompt := s.buildRunPrompt(ctx, issue, investigationID)
	name := runName(issue)

	sessionID, err := s.som.CreateRelaySession(ctx, bearer)
	if err != nil {
		return nil, somError(err)
	}
	repoID, err := s.som.EnsureRepo(ctx, sessionID)
	if err != nil {
		return nil, somError(err)
	}
	localEnvironmentID, err := s.som.StartEnvironment(ctx, sessionID, repoID, name, prompt)
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
		"som_environment_id", somEnvironmentID)

	return som.RunSomIssue201JSONResponse(som.SomIssueRunResult{
		IssueId:            request.IssueId,
		BoardId:            boardID,
		LocalEnvironmentId: localEnvUUID,
		SomEnvironmentId:   somEnvUUID,
		RepoId:             &repoUUID,
	}), nil
}

// buildRunPrompt дописывает к тексту issue контекст IR: адреса сервисов и
// идентификаторы, без которых агент не сможет вернуть находки в расследование.
func (s *Server) buildRunPrompt(ctx context.Context, issue somclient.Issue, investigationID string) string {
	var b strings.Builder
	b.WriteString(issue.Title)
	if issue.Description != nil && strings.TrimSpace(*issue.Description) != "" {
		b.WriteString("\n\n")
		b.WriteString(*issue.Description)
	}

	b.WriteString("\n\n---\nIR context (appended by ir-api):\n")
	writeLine := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- %s: %s\n", key, value)
		}
	}
	writeLine("ir_api_base_url", s.prompt.IRBaseURL)
	writeLine("gateway_base_url", s.prompt.GatewayBaseURL)
	if scope, ok := socctx.ScopeFromContext(ctx); ok {
		writeLine("project_id (send as X-Project-ID header)", scope.ProjectID)
	}
	writeLine("investigation_id", investigationID)
	writeLine("som_issue_id", issue.ID)

	b.WriteString("\nHow to search sources and report findings back to IR:\n" +
		"1. Search Gateway directly: POST {gateway_base_url}/api/v1/events/search with X-Project-ID. Keep source_code plus source_event_id/source_entity_id for every record you select.\n" +
		"2. Decide explicitly which findings belong on the graph. Give each selected event/entity a batch-local ref, then point nodes to it with event_ref or entity_ref. Edges address node source_ref/target_ref and cite event-node refs in evidence_event_refs. Every edge needs a non-empty why.\n" +
		"3. Submit exactly one batch: POST {ir_api_base_url}/api/v1/investigations/{investigation_id}/agent-results with X-Project-ID and body " +
		"{\"som_issue_ids\":[\"" + issue.ID + "\"],\"events\":[{\"ref\":\"selected-event-1\",\"source_code\":\"...\",\"source_event_id\":\"...\"}],\"entities\":[{\"ref\":\"selected-host-1\",\"source_code\":\"...\",\"source_entity_id\":\"...\"}],\"nodes\":[{\"ref\":\"event-1\",\"event_ref\":\"selected-event-1\"},{\"ref\":\"host-1\",\"entity_ref\":\"selected-host-1\"}],\"edges\":[{\"source_ref\":\"event-1\",\"target_ref\":\"host-1\",\"relation_code\":\"mentions\",\"why\":\"...\",\"confidence\":0.8,\"evidence_event_refs\":[\"event-1\"]}]}. IR resolves current normalized data from Gateway and draws only the listed nodes and edges; the endpoint assigns agent origin and proposed edge status. The batch is idempotent.\n" +
		"4. Verify the result with GET {ir_api_base_url}/api/v1/investigations/{investigation_id}/graph and /events.\n")
	if s.prompt.GatewayBaseURL != "" {
		b.WriteString("Gateway searches do not write to ir-api; only agent-results persists selected context.\n")
	}
	return b.String()
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
