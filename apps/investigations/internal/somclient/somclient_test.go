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

func TestConfigureInvestigationMCPPreservesExistingServers(t *testing.T) {
	t.Parallel()

	var posted struct {
		Servers map[string]json.RawMessage `json:"servers"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/config/mcp-config") || r.URL.Query().Get("executor") != "OPENCODE" {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"success":true,"data":{"mcp_config":{"servers":{"existing":{"type":"local","command":["tool"]}}}}}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatalf("decode update: %v", err)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":"updated"}`))
	}))
	defer server.Close()

	client := New(Config{RelayBaseURL: server.URL, HostID: "host", Executor: "OPENCODE"})
	if err := client.ConfigureInvestigationMCP(t.Context(), "session", "http://ir:8090/mcp", "project-1", "capability-token"); err != nil {
		t.Fatal(err)
	}
	if _, ok := posted.Servers["existing"]; !ok {
		t.Fatal("existing MCP server was dropped")
	}
	var investigation map[string]any
	if err := json.Unmarshal(posted.Servers["investigation"], &investigation); err != nil {
		t.Fatalf("decode investigation server: %v", err)
	}
	if investigation["url"] != "http://ir:8090/mcp" || investigation["type"] != "remote" {
		t.Fatalf("unexpected investigation config: %+v", investigation)
	}
	headers := investigation["headers"].(map[string]any)
	if headers["X-Project-ID"] != "project-1" {
		t.Fatalf("project header missing: %+v", headers)
	}
	if headers["X-Sb0rka-MCP-Token"] != "capability-token" {
		t.Fatalf("capability header missing: %+v", headers)
	}
}

func TestConfigureInvestigationMCPRejectsUnsupportedExecutor(t *testing.T) {
	t.Parallel()
	client := New(Config{Executor: "CODEX"})
	err := client.ConfigureInvestigationMCP(t.Context(), "session", "http://ir/mcp", "project", "token")
	if err == nil || !strings.Contains(err.Error(), "OPENCODE") {
		t.Fatalf("got %v, want OpenCode requirement", err)
	}
}
