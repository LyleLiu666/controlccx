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

func TestManager_run_ClaudeCode_ResumeNotFound_FailsWithMessage(t *testing.T) {
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
		Mode:       tasks.ModeResume,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo "No conversation found with session ID: sess-1" 1>&2`,
		"sleep 1",
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
	if !strings.Contains(strings.ToLower(updated.Error), "no conversation found") {
		t.Fatalf("error=%q, want contains %q", updated.Error, "no conversation found")
	}
	if !strings.Contains(strings.ToLower(updated.Warning), "resume failed") {
		t.Fatalf("warning=%q, want contains %q", updated.Warning, "resume failed")
	}
}
