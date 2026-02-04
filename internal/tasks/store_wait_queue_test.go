package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_DequeueNextWaitingForWorkdir_ClaimsFIFO(t *testing.T) {
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

	wait1, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:      WorkerExec,
		Mode:            ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create wait1: %v", err)
	}
	wait2, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:      WorkerExec,
		Mode:            ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 3",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create wait2: %v", err)
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

	next, ok, err := store.DequeueNextWaitingForWorkdir(ctx, ".")
	if err != nil {
		t.Fatalf("dequeue1: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if next.ID != wait1.ID {
		t.Fatalf("dequeue1 id=%q, want %q", next.ID, wait1.ID)
	}
	if next.Status != StatusQueued {
		t.Fatalf("dequeue1 status=%q, want %q", next.Status, StatusQueued)
	}

	_, ok, err = store.DequeueNextWaitingForWorkdir(ctx, ".")
	if err != nil {
		t.Fatalf("dequeue2: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false while a queued task exists")
	}

	if err := store.SetRunning(ctx, wait1.ID); err != nil {
		t.Fatalf("set running wait1: %v", err)
	}
	if err := store.FinishTask(ctx, wait1.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  "",
		FinishedAt: store.now().UTC(),
	}); err != nil {
		t.Fatalf("finish wait1: %v", err)
	}

	next2, ok, err := store.DequeueNextWaitingForWorkdir(ctx, ".")
	if err != nil {
		t.Fatalf("dequeue3: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true for second dequeue")
	}
	if next2.ID != wait2.ID {
		t.Fatalf("dequeue3 id=%q, want %q", next2.ID, wait2.ID)
	}
}

