package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_Invocation_SetAndGet(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := store.SetInvocation(ctx, task.ID, "claude", []string{"-p", "--verbose"}, "/x", []string{"ANTHROPIC_API_KEY"}); err != nil {
		t.Fatalf("SetInvocation: %v", err)
	}
	got, ok, err := store.GetInvocation(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetInvocation: %v", err)
	}
	if !ok {
		t.Fatalf("expected invocation to exist")
	}
	if got.Cmd != "claude" || got.Dir != filepath.Clean("/x") {
		t.Fatalf("got=%#v", got)
	}
	if len(got.Args) != 2 || got.Args[0] != "-p" {
		t.Fatalf("args=%v", got.Args)
	}
	if len(got.EnvInjectedKeys) != 1 || got.EnvInjectedKeys[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("env=%v", got.EnvInjectedKeys)
	}
}

