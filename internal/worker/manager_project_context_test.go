package worker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_buildToolCommand_PrefixesProjectContextForLLMWorkers(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	if _, err := store.SetProjectContext(ctx, "Project: CCX"); err != nil {
		t.Fatalf("SetProjectContext: %v", err)
	}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "P",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := NewManager(config.Default(), store, nil, nil, nil)

	tool, driver, err := m.buildToolCommand(ctx, task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	if driver != tasks.WorkerCodex {
		t.Fatalf("driver=%q, want %q", driver, tasks.WorkerCodex)
	}
	if !strings.HasPrefix(tool.Stdin, "Project Context:") {
		t.Fatalf("stdin=%q, want Project Context prefix", tool.Stdin)
	}
	if strings.Index(tool.Stdin, "Project: CCX") < 0 {
		t.Fatalf("stdin=%q, want context content included", tool.Stdin)
	}
	if strings.Index(tool.Stdin, "\n\n---\n\nP") < 0 {
		t.Fatalf("stdin=%q, want original prompt preserved after separator", tool.Stdin)
	}

	reloaded, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if reloaded.Prompt != "P" {
		t.Fatalf("stored prompt=%q, want %q (injection must not rewrite stored prompt)", reloaded.Prompt, "P")
	}
}
