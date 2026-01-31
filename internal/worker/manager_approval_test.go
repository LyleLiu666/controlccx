package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_ClaudeCode_RequiresApproval_BecomesBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: This command requires approval","is_error":true}]},"session_id":"sess-1"}'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}
	if !strings.Contains(strings.ToLower(updated.Warning), "approval") {
		t.Fatalf("warning=%q, want contains %q", updated.Warning, "approval")
	}
	if strings.TrimSpace(updated.Error) != "" {
		t.Fatalf("error=%q, want empty", updated.Error)
	}

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream != tasks.LogSystem {
			continue
		}
		if strings.Contains(strings.ToLower(l.Message), "requires approval") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected system log mentioning requires approval")
	}
}

func TestIsApprovalRequiredLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "requires_approval",
			line: `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: This command requires approval","is_error":true}]},"session_id":"sess-1"}`,
			want: true,
		},
		{
			name: "requested_permissions",
			line: `Claude requested permissions to write to index.html, but you haven't granted it yet.`,
			want: true,
		},
		{
			name: "normal_text",
			line: "boom",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isApprovalRequiredLine([]byte(tt.line)); got != tt.want {
				t.Fatalf("got=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestManager_run_ClaudeCode_NormalFailure_RemainsFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo '{"type":"assistant","session_id":"sess-1","result":"boom"}'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status=%q, want %q", updated.Status, tasks.StatusFailed)
	}
}

func TestManager_run_ClaudeCode_RequestedPermissions_BecomesBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo 'Claude requested permissions to write to index.html, but you haven'"'"'t granted it yet.'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}
}
