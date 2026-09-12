package somclient

import (
	"context"
	"encoding/json"
	"io"
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

func TestUpdateIssue(t *testing.T) {
	t.Parallel()

	issueID := "11111111-1111-4111-8111-111111111111"
	boardID := "22222222-2222-4222-8222-222222222222"

	for _, tc := range []struct {
		name        string
		description *string
		wantBody    string
	}{
		{name: "set description", description: ptr("new text"), wantBody: `{"description":"new text"}`},
		{name: "clear description", description: nil, wantBody: `{"description":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Fatalf("method: %s", r.Method)
				}
				if r.URL.Path != "/v1/issues/"+issueID {
					t.Fatalf("path: %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Fatalf("authorization: %q", got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(string(body)) != tc.wantBody {
					t.Fatalf("body: got %s, want %s", body, tc.wantBody)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"id":           issueID,
						"board_id":     boardID,
						"issue_number": 7,
						"simple_id":    "IR-7",
						"title":        "Task",
						"description":  tc.description,
					},
					"txid": 42,
				})
			}))
			defer upstream.Close()

			client := New(Config{APIBaseURL: upstream.URL})
			got, err := client.UpdateIssue(context.Background(), "test-token", issueID, IssuePatch{
				Description: tc.description,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != issueID || got.SimpleID != "IR-7" || got.Title != "Task" {
				t.Fatalf("issue: %+v", got)
			}
			if tc.description == nil {
				if got.Description != nil {
					t.Fatalf("description should be nil: %+v", got.Description)
				}
			} else if got.Description == nil || *got.Description != *tc.description {
				t.Fatalf("description: %+v", got.Description)
			}
		})
	}
}
