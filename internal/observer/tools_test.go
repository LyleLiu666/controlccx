package observer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestService_resolveTaskID_ByPromptKeyword(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task1, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "写一个脱口秀段子，至少 1500 字",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task1: %v", err)
	}
	exitCode := 0
	if err := store.FinishTask(ctx, task1.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task1: %v", err)
	}

	_, err = store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "写一首诗，至少 200 字",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task2: %v", err)
	}

	svc := &Service{Store: store}
	got, err := svc.resolveTaskID(ctx, "脱口秀")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != task1.ID {
		t.Fatalf("got=%q, want %q", got, task1.ID)
	}
}

func TestService_resolveTaskID_AmbiguousPromptKeyword(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task1, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "写一个脱口秀段子 A",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task1: %v", err)
	}
	exitCode := 0
	if err := store.FinishTask(ctx, task1.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task1: %v", err)
	}
	_, err = store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "写一个脱口秀段子 B",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task2: %v", err)
	}

	svc := &Service{Store: store}
	_, err = svc.resolveTaskID(ctx, "脱口秀")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err=%q, want contains %q", err.Error(), "ambiguous")
	}
}
