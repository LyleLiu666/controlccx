package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_TaskLifecycleAndScoring(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	fixedNow := time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixedNow }

	_, err = store.CreateTask(ctx, CreateTaskInput{Prompt: "x", WorkDir: "."})
	if err == nil {
		t.Fatalf("expected error for missing worker_type")
	}

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != StatusQueued {
		t.Fatalf("status=%q, want %q", task.Status, StatusQueued)
	}

	if err := store.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	task, err = store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if task.Status != StatusRunning {
		t.Fatalf("status=%q, want %q", task.Status, StatusRunning)
	}
	if task.StartedAt == nil {
		t.Fatalf("expected started_at set")
	}

	if _, err := store.AppendLog(ctx, task.ID, LogStdout, "all good"); err != nil {
		t.Fatalf("append stdout: %v", err)
	}
	task, _ = store.GetTask(ctx, task.ID)
	if task.Score != 0 {
		t.Fatalf("score=%d, want 0", task.Score)
	}

	if _, err := store.AppendLog(ctx, task.ID, LogStderr, "something failed"); err != nil {
		t.Fatalf("append stderr: %v", err)
	}
	task, _ = store.GetTask(ctx, task.ID)
	if task.StderrCount != 1 {
		t.Fatalf("stderr_count=%d, want 1", task.StderrCount)
	}
	if task.KeywordCount != 1 {
		t.Fatalf("keyword_count=%d, want 1", task.KeywordCount)
	}
	if task.Score <= 0 {
		t.Fatalf("expected score > 0, got %d", task.Score)
	}

	exitCode := 1
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusFailed,
		ExitCode:   &exitCode,
		Error:      "non-zero exit",
		SessionID:  "sess-123",
		FinishedAt: fixedNow,
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	task, _ = store.GetTask(ctx, task.ID)
	if task.Status != StatusFailed {
		t.Fatalf("status=%q, want %q", task.Status, StatusFailed)
	}
	if task.ExitCode == nil || *task.ExitCode != 1 {
		t.Fatalf("exit_code=%v, want 1", task.ExitCode)
	}
	if task.SessionID != "sess-123" {
		t.Fatalf("session_id=%q, want sess-123", task.SessionID)
	}
	if task.FinishedAt == nil {
		t.Fatalf("expected finished_at set")
	}
	if task.Score < nonZeroExitScore {
		t.Fatalf("expected score includes non-zero exit, got %d", task.Score)
	}

	task2, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Prompt:     "sleep",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task2: %v", err)
	}
	if err := store.SetRunning(ctx, task2.ID); err != nil {
		t.Fatalf("set running task2: %v", err)
	}
	updated, err := store.MarkInterrupted(ctx)
	if err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if updated < 1 {
		t.Fatalf("expected at least 1 updated, got %d", updated)
	}
	task2, _ = store.GetTask(ctx, task2.ID)
	if task2.Status != StatusInterrupted {
		t.Fatalf("task2 status=%q, want %q", task2.Status, StatusInterrupted)
	}
}

func TestStore_ListLogs(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	e1, err := store.AppendLog(ctx, task.ID, LogStdout, "a")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, LogStdout, "b")

	logs, err := store.ListLogs(ctx, task.ID, e1.ID, 10)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len=%d, want 1", len(logs))
	}
	if logs[0].Message != "b" {
		t.Fatalf("message=%q, want b", logs[0].Message)
	}
}

