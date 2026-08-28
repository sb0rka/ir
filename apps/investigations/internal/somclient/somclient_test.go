package somclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ptr(value string) *string { return &value }

func TestResolveExecutorConfig(t *testing.T) {
	t.Parallel()

	got := ResolveExecutorConfig(nil, nil)
	if got.ModelID != DefaultModelID {
		t.Fatalf("default model_id: got %q, want %q", got.ModelID, DefaultModelID)
	}
	if got.Variant != "" {
		t.Fatalf("default variant: got %q, want empty", got.Variant)
	}

	got = ResolveExecutorConfig(ptr(" PLAN "), ptr(" openrouter/other "))
	if got.Variant != "PLAN" {
		t.Fatalf("variant: got %q, want PLAN", got.Variant)
	}
	if got.ModelID != "openrouter/other" {
		t.Fatalf("model_id: got %q, want openrouter/other", got.ModelID)
	}

	got = ResolveExecutorConfig(ptr("  "), ptr("  "))
	if got.ModelID != DefaultModelID || got.Variant != "" {
		t.Fatalf("blank strings should fall back to defaults: %+v", got)
	}
}

func TestStatusFromLatestProcess(t *testing.T) {
	t.Parallel()

	running := "running"
	failed := "failed"
	killed := "killed"
	completed := "completed"
	blank := "  "

	cases := []struct {
		name string
		in   *string
		want EnvironmentStatus
	}{
		{name: "nil means still starting", in: nil, want: EnvironmentStatus{IsRunning: true}},
		{name: "blank means still starting", in: &blank, want: EnvironmentStatus{IsRunning: true}},
		{name: "running", in: &running, want: EnvironmentStatus{IsRunning: true}},
		{name: "failed", in: &failed, want: EnvironmentStatus{IsErrored: true}},
		{name: "killed", in: &killed, want: EnvironmentStatus{IsErrored: true}},
		{name: "completed", in: &completed, want: EnvironmentStatus{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := statusFromLatestProcess(tc.in)
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestExecutorConfigPayload(t *testing.T) {
	t.Parallel()

	payload := executorConfigPayload("OPENCODE", ResolveExecutorConfig(nil, nil))
	if payload["executor"] != "OPENCODE" {
		t.Fatalf("executor: %+v", payload)
	}
	if payload["model_id"] != DefaultModelID {
		t.Fatalf("model_id: %+v", payload)
	}
	if _, ok := payload["variant"]; ok {
		t.Fatalf("variant should be omitted by default: %+v", payload)
	}

	payload = executorConfigPayload("OPENCODE", ExecutorConfig{Variant: "DEFAULT", ModelID: "openrouter/x"})
	if payload["variant"] != "DEFAULT" || payload["model_id"] != "openrouter/x" {
		t.Fatalf("overrides: %+v", payload)
	}
}

func TestStartEnvironmentIncludesRemoteMCPServersInOneRequest(t *testing.T) {
	t.Parallel()

	var posted map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/environments/start") || r.Method != http.MethodPost {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatalf("decode start: %v", err)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"environment":{"id":"environment-1"},"execution_process":{}}}`))
	}))
	defer server.Close()

	client := New(Config{RelayBaseURL: server.URL, HostID: "host", Executor: "OPENCODE", TargetBranch: "main"})
	servers := map[string]RemoteMCPServer{
		"investigation": {
			URL:     "http://ir:8090/mcp",
			Enabled: true,
			Headers: map[string]string{"X-Sb0rka-MCP-Token": "capability-token"},
		},
	}
	environmentID, err := client.StartEnvironment(
		t.Context(), "session", "repo", "name", "prompt", ExecutorConfig{}, servers)
	if err != nil {
		t.Fatal(err)
	}
	if environmentID != "environment-1" {
		t.Fatalf("environment id: %q", environmentID)
	}
	var remote map[string]RemoteMCPServer
	if err := json.Unmarshal(posted["remote_mcp_servers"], &remote); err != nil {
		t.Fatalf("decode remote MCP servers: %v", err)
	}
	if remote["investigation"].URL != "http://ir:8090/mcp" ||
		remote["investigation"].Headers["X-Sb0rka-MCP-Token"] != "capability-token" {
		t.Fatalf("unexpected investigation config: %+v", remote)
	}
}

func TestStartEnvironmentRejectsRemoteMCPForUnsupportedExecutor(t *testing.T) {
	t.Parallel()
	client := New(Config{Executor: "CODEX"})
	_, err := client.StartEnvironment(t.Context(), "session", "repo", "name", "prompt", ExecutorConfig{}, map[string]RemoteMCPServer{
		"investigation": {URL: "http://ir/mcp", Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "OPENCODE") {
		t.Fatalf("got %v, want OpenCode requirement", err)
	}
}
