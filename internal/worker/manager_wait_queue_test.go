package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_StartsNextWaitingTaskForWorkdir(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo A",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	waiting, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo B",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}
	if waiting.Status != tasks.StatusWaiting {
		t.Fatalf("waiting status=%q, want %q", waiting.Status, tasks.StatusWaiting)
	}

	m := &Manager{
		cfg:   config.Default(),
		store: store,
	}

	if err := m.run(ctx, first); err != nil {
		t.Fatalf("run first: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := store.GetTask(ctx, waiting.ID)
		if err != nil {
			t.Fatalf("get waiting: %v", err)
		}
		if got.Status == tasks.StatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for queued task to start and finish; status=%q err=%q", got.Status, got.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

