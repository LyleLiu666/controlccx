package secretary

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestTools_TasksCount_RejectsUnknownStatus(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	reg := newToolRegistry(taskStore)

	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "tasks_count",
		Fields: map[string]string{"status": "not-a-status"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTools_TasksList_TruncatesUTF8Safely(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	long := strings.Repeat("中", 300)
	if _, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     long,
		WorkDir:    ".",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	reg := newToolRegistry(taskStore)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "tasks_list",
		Fields: map[string]string{"limit": "1", "include_deleted": "1"},
	})
	if err != nil {
		t.Fatalf("execute tasks_list: %v", err)
	}
	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type %T", outAny)
	}
	list, ok := out["tasks"].([]taskSummary)
	if !ok || len(list) != 1 {
		t.Fatalf("unexpected tasks payload: %#v", out["tasks"])
	}

	got := list[0].Prompt
	want := strings.Repeat("中", 239) + "…"
	if got != want {
		t.Fatalf("prompt truncated mismatch: got=%q want=%q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid utf-8, got: %q", got)
	}
}
