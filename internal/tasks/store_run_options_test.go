package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_CreateTask_PersistsUnsafeAutomationOption(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:       WorkerClaudeCode,
		Mode:             ModeNew,
		Prompt:           "hi",
		WorkDir:          ".",
		UnsafeAutomation: true,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if !task.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want true", task.UnsafeAutomation)
	}

	loaded, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !loaded.UnsafeAutomation {
		t.Fatalf("loaded unsafe_automation=%v, want true", loaded.UnsafeAutomation)
	}

	list, err := store.ListTasks(ctx, 50)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	found := false
	for _, it := range list {
		if it.ID != task.ID {
			continue
		}
		found = true
		if !it.UnsafeAutomation {
			t.Fatalf("listed unsafe_automation=%v, want true", it.UnsafeAutomation)
		}
	}
	if !found {
		t.Fatalf("task not found in list")
	}
}

