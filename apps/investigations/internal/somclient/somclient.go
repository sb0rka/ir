// Package somclient — HTTP-клиент демо-интеграции с SOM: kanban API самого
// SOM, relay-сессии и daemon-хост за relay. Bearer для SOM приходит из
// project-scoped Secrets через server layer; daemon-пути аутентифицируются
// relay-сессией в самом URL.
package somclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	// APIBaseURL — SOM backend, например http://localhost:3000. Пустое
	// значение означает «интеграция не сконфигурирована»: som-роуты отвечают
	// 502 с понятным текстом, остальной сервис живёт как жил.
	APIBaseURL string
	// RelayBaseURL — relay, через который достижим daemon-хост.
	RelayBaseURL string
	// HostID — единственная ручная daemon VM демо-стенда.
	HostID string
	// RepoID — заранее зарегистрированный репозиторий на daemon. Пустое
	// значение включает EnsureRepo: найти по имени папки или git init.
	RepoID         string
	RepoParentPath string
	RepoFolderName string
	TargetBranch   string
	// Executor — исполнитель агента в терминах daemon, например OPENCODE.
	Executor string
}

// DefaultModelID — OpenRouter DeepSeek V4 Flash, формат OpenCode provider/model.
const DefaultModelID = "openrouter/deepseek/deepseek-v4-flash"

// ExecutorConfig — поля daemon executor_config, которые IR пробрасывает
// из тела POST /som/issues/{id}/run. Executor берётся из конфига клиента.
type ExecutorConfig struct {
	Variant string
	ModelID string
}

// ResolveExecutorConfig подставляет default model, если клиент его не задал.
func ResolveExecutorConfig(variant, modelID *string) ExecutorConfig {
	out := ExecutorConfig{ModelID: DefaultModelID}
	if value := optionalString(variant); value != "" {
		out.Variant = value
	}
	if value := optionalString(modelID); value != "" {
		out.ModelID = value
	}
	return out
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		// Старт окружения тянет за собой git-операции на хосте — минута
		// с запасом, обычные вызовы укладываются в секунды.
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) Configured() bool { return strings.TrimSpace(c.cfg.APIBaseURL) != "" }

// RunConfigured проверяет всё, что нужно именно для запуска issue.
func (c *Client) RunConfigured() error {
	switch {
	case !c.Configured():
		return fmt.Errorf("SOM_API_BASE_URL is not set")
	case strings.TrimSpace(c.cfg.RelayBaseURL) == "":
		return fmt.Errorf("SOM_RELAY_BASE_URL is not set")
	case strings.TrimSpace(c.cfg.HostID) == "":
		return fmt.Errorf("SOM_HOST_ID is not set")
	}
	return nil
}

// UpstreamError — не-2xx ответ SOM/relay/daemon. Тело сохраняется: на демо
// диагноз важнее, чем стерильный ответ.
type UpstreamError struct {
	Op     string
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("%s: upstream status %d: %s", e.Op, e.Status, e.Body)
}

// --- Типы SOM API (подмножество полей, которые использует IR) ---

type Workspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	IsPersonal  bool   `json:"is_personal"`
	IssuePrefix string `json:"issue_prefix"`
	UserRole    string `json:"user_role"`
}

type Board struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

type Issue struct {
	ID            string  `json:"id"`
	BoardID       string  `json:"board_id"`
	IssueNumber   int     `json:"issue_number"`
	SimpleID      string  `json:"simple_id"`
	Title         string  `json:"title"`
	Description   *string `json:"description"`
	Priority      *string `json:"priority"`
	ParentIssueID *string `json:"parent_issue_id"`
}

func (c *Client) ListWorkspaces(ctx context.Context, bearer string) ([]Workspace, error) {
	var out struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	err := c.doJSON(ctx, "som list workspaces", http.MethodGet,
		c.cfg.APIBaseURL+"/v1/workspaces", bearer, nil, &out)
	return out.Workspaces, err
}

func (c *Client) ListBoards(ctx context.Context, bearer, workspaceID string) ([]Board, error) {
	var out struct {
		Boards []Board `json:"boards"`
	}
	err := c.doJSON(ctx, "som list boards", http.MethodGet,
		c.cfg.APIBaseURL+"/v1/boards?workspace_id="+url.QueryEscape(workspaceID), bearer, nil, &out)
	return out.Boards, err
}

func (c *Client) ListIssues(ctx context.Context, bearer, boardID string) ([]Issue, int, error) {
	var out struct {
		Issues     []Issue `json:"issues"`
		TotalCount int     `json:"total_count"`
	}
	err := c.doJSON(ctx, "som list issues", http.MethodGet,
		c.cfg.APIBaseURL+"/v1/issues?board_id="+url.QueryEscape(boardID), bearer, nil, &out)
	return out.Issues, out.TotalCount, err
}

func (c *Client) GetIssue(ctx context.Context, bearer, issueID string) (Issue, error) {
	var out Issue
	err := c.doJSON(ctx, "som get issue", http.MethodGet,
		c.cfg.APIBaseURL+"/v1/issues/"+url.PathEscape(issueID), bearer, nil, &out)
	return out, err
}

// LinkEnvironment привязывает daemon-окружение к issue на стороне SOM —
// последний шаг запуска, после него запуск виден в UI SOM.
func (c *Client) LinkEnvironment(ctx context.Context, bearer, boardID, issueID, localEnvironmentID, name string) (string, error) {
	body := map[string]any{
		"board_id":             boardID,
		"local_environment_id": localEnvironmentID,
		"issue_id":             issueID,
		"name":                 name,
		"archived":             false,
	}
	var out struct {
		ID string `json:"id"`
	}
	err := c.doJSON(ctx, "som link environment", http.MethodPost,
		c.cfg.APIBaseURL+"/v1/environments", bearer, body, &out)
	return out.ID, err
}

// --- Relay и daemon ---

// CreateRelaySession открывает браузерную relay-сессию к daemon-хосту.
// Дальше сессия живёт в пути URL, bearer daemon'у не передаётся.
func (c *Client) CreateRelaySession(ctx context.Context, bearer string) (string, error) {
	var out struct {
		SessionID string `json:"session_id"`
	}
	err := c.doJSON(ctx, "relay create session", http.MethodPost,
		c.cfg.RelayBaseURL+"/v1/relay/create/"+url.PathEscape(c.cfg.HostID), bearer, nil, &out)
	return out.SessionID, err
}

func (c *Client) relayURL(sessionID, daemonPath string) string {
	return c.cfg.RelayBaseURL + "/v1/relay/h/" + url.PathEscape(c.cfg.HostID) +
		"/s/" + url.PathEscape(sessionID) + daemonPath
}

type Repo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// EnsureRepo возвращает репозиторий для окружения: заданный конфигом, уже
// существующий с именем RepoFolderName, либо свежесозданный git init'ом на
// хосте. Daemon отказывает пустому списку repos, поэтому какой-то нужен всегда.
func (c *Client) EnsureRepo(ctx context.Context, sessionID string) (string, error) {
	if repoID := strings.TrimSpace(c.cfg.RepoID); repoID != "" {
		return repoID, nil
	}

	var repos []Repo
	if err := c.doDaemon(ctx, "daemon list repos", http.MethodGet,
		c.relayURL(sessionID, "/api/repos"), nil, &repos); err != nil {
		return "", err
	}
	for _, repo := range repos {
		if repo.Name == c.cfg.RepoFolderName {
			return repo.ID, nil
		}
	}

	var created Repo
	err := c.doDaemon(ctx, "daemon init repo", http.MethodPost,
		c.relayURL(sessionID, "/api/repos/init"), map[string]any{
			"parent_path": c.cfg.RepoParentPath,
			"folder_name": c.cfg.RepoFolderName,
		}, &created)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// StartEnvironment создаёт и запускает окружение с агентом. linked_issue не
// передаётся сознательно — фронтенд SOM делает так же, а связь с issue
// оформляется на стороне SOM через LinkEnvironment.
func (c *Client) StartEnvironment(ctx context.Context, sessionID, repoID, name, prompt string, exec ExecutorConfig) (string, error) {
	body := map[string]any{
		"name": name,
		"repos": []map[string]any{{
			"repo_id":       repoID,
			"target_branch": c.cfg.TargetBranch,
		}},
		"linked_issue":    nil,
		"executor_config": executorConfigPayload(c.cfg.Executor, exec),
		"prompt":          prompt,
		"attachment_ids":  nil,
	}
	var out struct {
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
	}
	err := c.doDaemon(ctx, "daemon start environment", http.MethodPost,
		c.relayURL(sessionID, "/api/environments/start"), body, &out)
	return out.Environment.ID, err
}

// EnvironmentStatus — подмножество daemon EnvironmentWithStatus для polling.
type EnvironmentStatus struct {
	ID        string
	IsRunning bool
	IsErrored bool
}

// GetEnvironment читает статус окружения на daemon через relay-сессию.
//
// Важно: `GET /api/environments/{id}` отдаёт plain Environment без
// `is_running`/`is_errored`. Парсить его нельзя — нули выглядят как
// completed, пока агент ещё работает. Статус берём из
// `POST /api/environments/summaries` → `latest_process_status` (то же
// семейство процессов, что считает `is_running` в WS-стриме SOM).
func (c *Client) GetEnvironment(ctx context.Context, sessionID, environmentID string) (EnvironmentStatus, error) {
	for _, archived := range []bool{false, true} {
		status, found, err := c.lookupEnvironmentSummary(ctx, sessionID, environmentID, archived)
		if err != nil {
			return EnvironmentStatus{}, err
		}
		if found {
			status.ID = environmentID
			return status, nil
		}
	}
	return EnvironmentStatus{}, &UpstreamError{
		Op:     "daemon get environment",
		Status: http.StatusNotFound,
		Body:   "environment not found in daemon summaries",
	}
}

func (c *Client) lookupEnvironmentSummary(
	ctx context.Context, sessionID, environmentID string, archived bool,
) (EnvironmentStatus, bool, error) {
	var out struct {
		Summaries []struct {
			EnvironmentID       string  `json:"environment_id"`
			LatestProcessStatus *string `json:"latest_process_status"`
		} `json:"summaries"`
	}
	err := c.doDaemon(ctx, "daemon environment summaries", http.MethodPost,
		c.relayURL(sessionID, "/api/environments/summaries"),
		map[string]any{"archived": archived}, &out)
	if err != nil {
		return EnvironmentStatus{}, false, err
	}
	for _, summary := range out.Summaries {
		if !strings.EqualFold(summary.EnvironmentID, environmentID) {
			continue
		}
		return statusFromLatestProcess(summary.LatestProcessStatus), true, nil
	}
	return EnvironmentStatus{}, false, nil
}

// statusFromLatestProcess: нет процесса → ещё стартует (running);
// failed/killed → errored; completed → idle.
func statusFromLatestProcess(latest *string) EnvironmentStatus {
	if latest == nil || strings.TrimSpace(*latest) == "" {
		return EnvironmentStatus{IsRunning: true}
	}
	switch strings.ToLower(strings.TrimSpace(*latest)) {
	case "running":
		return EnvironmentStatus{IsRunning: true}
	case "failed", "killed":
		return EnvironmentStatus{IsErrored: true}
	default:
		return EnvironmentStatus{}
	}
}

func executorConfigPayload(executor string, exec ExecutorConfig) map[string]any {
	payload := map[string]any{
		"executor": executor,
		"model_id": exec.ModelID,
	}
	if exec.Variant != "" {
		payload["variant"] = exec.Variant
	}
	return payload
}

// --- HTTP ---

const maxResponseBody = 1 << 20

func (c *Client) doJSON(ctx context.Context, op, method, rawURL, bearer string, in, out any) error {
	var payload io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("%s: encode request: %w", op, err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, payload)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &UpstreamError{Op: op, Status: 0, Body: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return &UpstreamError{Op: op, Status: resp.StatusCode, Body: "read body: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &UpstreamError{Op: op, Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &UpstreamError{Op: op, Status: resp.StatusCode, Body: "decode response: " + err.Error()}
	}
	return nil
}

// doDaemon разворачивает конверт daemon {success, data, message}.
func (c *Client) doDaemon(ctx context.Context, op, method, rawURL string, in, out any) error {
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Message *string         `json:"message"`
	}
	if err := c.doJSON(ctx, op, method, rawURL, "", in, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		message := "daemon reported failure"
		if envelope.Message != nil && *envelope.Message != "" {
			message = *envelope.Message
		}
		return &UpstreamError{Op: op, Status: 0, Body: message}
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return &UpstreamError{Op: op, Status: 0, Body: "decode daemon data: " + err.Error()}
	}
	return nil
}
