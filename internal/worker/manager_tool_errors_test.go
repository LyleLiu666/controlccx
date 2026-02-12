package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_ClaudeCode_ToolResultError_SetsWarningAndEmitsStderr(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_PROCESS", "1")
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_TOOL_ERROR", "1")

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

	cfg := config.Default()
	cfg.Paths.Claude = os.Args[0]

	m := NewManager(cfg, store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if updated.Status != tasks.StatusSucceeded {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusSucceeded, updated.Warning, updated.Error)
	}
	if !strings.Contains(updated.Warning, "tool errors were observed") {
		t.Fatalf("warning=%q, want contains %q", updated.Warning, "tool errors were observed")
	}

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	foundToolError := false
	for _, e := range logs {
		if e.Stream != tasks.LogStderr {
			continue
		}
		if !strings.HasPrefix(e.Message, "tool_error:") {
			continue
		}
		foundToolError = true
		if !strings.Contains(e.Message, "Operation not permitted") {
			t.Fatalf("tool_error=%q, want contains %q", e.Message, "Operation not permitted")
		}
	}
	if !foundToolError {
		t.Fatalf("expected tool_error stderr log, logs=%v", logs)
	}
}
