package worker

import (
	"strings"
	"testing"

	"controlccx/internal/tasks"
)

func TestShouldUseRunWorkspace_ByIntent(t *testing.T) {
	cases := []struct {
		name        string
		task        tasks.Task
		hasExisting bool
		initProject bool
		wantUse     bool
	}{
		{
			name:    "code uses workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: "code"},
			wantUse: true,
		},
		{
			name:    "install uses workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerCodex, TaskIntent: "install"},
			wantUse: true,
		},
		{
			name:    "analyze skips workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerCodex, TaskIntent: "analyze"},
			wantUse: false,
		},
		{
			name:    "search-browse skips workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: "search-browse"},
			wantUse: false,
		},
		{
			name:    "exec uses workspace by default",
			task:    tasks.Task{WorkerType: tasks.WorkerExec, TaskIntent: ""},
			wantUse: true,
		},
		{
			name:        "existing workspace always used",
			task:        tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: "search-browse"},
			hasExisting: true,
			initProject: true,
			wantUse:     true,
		},
		{
			name:        "init project skips workspace for new workspaces",
			task:        tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: "code"},
			initProject: true,
			wantUse:     false,
		},
		{
			name:    "unknown intent defaults to workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: "weird"},
			wantUse: true,
		},
		{
			name:    "empty intent defaults to workspace",
			task:    tasks.Task{WorkerType: tasks.WorkerClaudeCode, TaskIntent: ""},
			wantUse: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := shouldUseRunWorkspace(tc.task, tc.hasExisting, tc.initProject)
			if got != tc.wantUse {
				t.Fatalf("use=%v, want %v (reason=%q)", got, tc.wantUse, reason)
			}
			if strings.TrimSpace(reason) == "" {
				t.Fatalf("expected non-empty reason")
			}
		})
	}
}
