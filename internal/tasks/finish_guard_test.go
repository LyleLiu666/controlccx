package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_FinishTask_DoesNotOverrideDifferentTerminalStatus(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	t1 := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusInterrupted,
		ExitCode:   nil,
		Error:      "watchdog",
		SessionID:  "",
		FinishedAt: t1,
	}); err != nil {
		t.Fatalf("finish interrupted: %v", err)
	}

	// A later finish (e.g. runner completing) must not override the terminal interrupted state.
	t2 := t1.Add(2 * time.Minute)
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: t2,
	}); err != nil {
		t.Fatalf("finish succeeded: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != StatusInterrupted {
		t.Fatalf("status=%q, want %q", updated.Status, StatusInterrupted)
	}
	if updated.FinishedAt == nil || !updated.FinishedAt.UTC().Equal(t1) {
		t.Fatalf("finished_at=%v, want %v", updated.FinishedAt, t1)
	}
}

