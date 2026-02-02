package tasks

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_CreateTask_RejectsWhenWorkdirBusy(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	first, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	_, err = store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo 2",
		WorkDir:    ".",
	})
	var busy *WorkDirBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("err=%v, want WorkDirBusyError", err)
	}
	if busy.ExistingTaskID != first.ID {
		t.Fatalf("existing_task_id=%q want %q", busy.ExistingTaskID, first.ID)
	}
	if busy.ExistingStatus != StatusQueued {
		t.Fatalf("existing_status=%q want %q", busy.ExistingStatus, StatusQueued)
	}
}

func TestStore_CreateTask_IdempotencyBypassesWorkdirBusy(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	first, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerExec,
		Mode:           ModeNew,
		IdempotencyKey: "k-1",
		Prompt:         "echo 1",
		WorkDir:        ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	second, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerExec,
		Mode:           ModeNew,
		IdempotencyKey: "k-1",
		Prompt:         "echo 2",
		WorkDir:        ".",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("id mismatch: first=%s second=%s", first.ID, second.ID)
	}
}
