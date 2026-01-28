package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestManager_appendLog_PublishesTaskUpdated(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	hub := events.NewHub()
	m := NewManager(config.Default(), store, hub, nil, nil)

	ch, unsub := hub.Subscribe(16)
	t.Cleanup(unsub)

	m.appendLog(task.ID, tasks.LogSystem, "hello")

	gotLog := false
	gotUpdated := false

	deadline := time.After(1 * time.Second)
	for !(gotLog && gotUpdated) {
		select {
		case evt := <-ch:
			switch evt.Type {
			case "task.log":
				gotLog = true
			case "task.updated":
				gotUpdated = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting for events (gotLog=%v gotUpdated=%v)", gotLog, gotUpdated)
		}
	}
}
