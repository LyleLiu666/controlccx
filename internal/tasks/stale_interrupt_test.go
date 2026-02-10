package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_InterruptTaskIfStaleInFlight_UpdatesOnlyWhenStaleAndInFlight(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	staleBefore := now.Add(-15 * time.Minute)

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
	// Force staleness beyond the threshold.
	if _, err := conn.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, toMillis(staleBefore.Add(-time.Minute)), task.ID); err != nil {
		t.Fatalf("set old updated_at: %v", err)
	}

	ok, err := store.InterruptTaskIfStaleInFlight(ctx, task.ID, staleBefore, now, "stale watchdog timeout")
	if err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	if !ok {
		t.Fatalf("interrupt ok=false, want true")
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != StatusInterrupted {
		t.Fatalf("status=%q, want %q", updated.Status, StatusInterrupted)
	}
	if updated.FinishedAt == nil {
		t.Fatalf("finished_at is nil; want set")
	}
	if updated.Error == "" {
		t.Fatalf("error is empty; want watchdog reason")
	}
}

func TestStore_InterruptTaskIfStaleInFlight_SkipsWhenFresh(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	staleBefore := now.Add(-15 * time.Minute)

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
	// Make the task look fresh.
	if _, err := conn.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, toMillis(now), task.ID); err != nil {
		t.Fatalf("set fresh updated_at: %v", err)
	}

	ok, err := store.InterruptTaskIfStaleInFlight(ctx, task.ID, staleBefore, now, "stale watchdog timeout")
	if err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	if ok {
		t.Fatalf("interrupt ok=true, want false")
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != StatusRunning {
		t.Fatalf("status=%q, want %q", updated.Status, StatusRunning)
	}
}

