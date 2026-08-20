package somclient

import (
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
