package worker

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_Start_RejectsWaitingTasks(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	_, err = store.CreateTask(ctx, tasks.CreateTaskInput{
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

	m := &Manager{cfg: config.Default(), store: store}
	if err := m.Start(ctx, waiting.ID); err == nil {
		t.Fatalf("expected Start to reject waiting task")
	}

	got, err := store.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("get waiting: %v", err)
	}
	if got.Status != tasks.StatusWaiting {
		t.Fatalf("status=%q, want %q", got.Status, tasks.StatusWaiting)
	}
}

