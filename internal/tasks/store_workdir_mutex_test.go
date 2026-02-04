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

func TestStore_CreateTask_WaitStrategy_AllowsBusyWorkdirAndMarksWaiting(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	_, err = store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	second, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:       WorkerExec,
		Mode:             ModeNew,
		WorkDirStrategy:  "wait",
		Prompt:           "echo 2",
		WorkDir:          ".",
		IdempotencyKey:   "k-2",
		UnsafeAutomation: false,
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.Status != StatusWaiting {
		t.Fatalf("second status=%q, want %q", second.Status, StatusWaiting)
	}
}

func TestStore_CreateTask_ExclusiveRejectedWhenWaitingPresent(t *testing.T) {
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
		WorkerType:      WorkerExec,
		Mode:            ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}

	exitCode := 0
	if err := store.FinishTask(ctx, first.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  "",
		FinishedAt: store.now().UTC(),
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}

	_, err = store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo 3",
		WorkDir:    ".",
	})
	var busy *WorkDirBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("err=%v, want WorkDirBusyError", err)
	}
	if busy.ExistingStatus != StatusWaiting {
		t.Fatalf("existing_status=%q want %q", busy.ExistingStatus, StatusWaiting)
	}
}
