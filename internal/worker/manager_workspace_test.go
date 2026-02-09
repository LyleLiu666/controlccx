package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_InitProject_ExecutesInBaseWorkdir(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	base := t.TempDir()

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hi > marker.txt",
		WorkDir:    base,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := NewManager(config.Default(), store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Empty, non-git directories are treated as init projects: the command writes into the base workdir.
	if _, err := os.Stat(filepath.Join(base, "marker.txt")); err != nil {
		t.Fatalf("expected base marker.txt created: %v", err)
	}

	key := tasks.SessionKeyForTask(task)
	if _, ok, err := store.GetSessionWorkspace(ctx, key); err != nil || ok {
		t.Fatalf("expected no session workspace for init project; ok=%v err=%v", ok, err)
	}

	inv, ok, err := store.GetInvocation(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("expected invocation; ok=%v err=%v", ok, err)
	}
	if filepath.Clean(inv.Dir) != filepath.Clean(base) {
		t.Fatalf("invocation dir=%q, want %q", inv.Dir, base)
	}
}

func TestManager_run_ExecutesInRunWorkspaceByDefault_ForNonEmptyWorkdir(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "dummy.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dummy: %v", err)
	}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hi > marker.txt",
		WorkDir:    base,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := NewManager(config.Default(), store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	// The command should not write into the base workdir.
	if _, err := os.Stat(filepath.Join(base, "marker.txt")); err == nil {
		t.Fatalf("expected base marker.txt not created")
	}

	key := tasks.SessionKeyForTask(task)
	ws, ok, err := store.GetSessionWorkspace(ctx, key)
	if err != nil || !ok {
		t.Fatalf("expected session workspace; ok=%v err=%v", ok, err)
	}
	if ws.RunWorkDir == "" {
		t.Fatalf("expected run_workdir set")
	}
	if _, err := os.Stat(filepath.Join(ws.RunWorkDir, "marker.txt")); err != nil {
		t.Fatalf("expected marker.txt in run workspace: %v", err)
	}

	inv, ok, err := store.GetInvocation(ctx, task.ID)
	if err != nil || !ok {
		t.Fatalf("expected invocation; ok=%v err=%v", ok, err)
	}
	if filepath.Clean(inv.Dir) != filepath.Clean(ws.RunWorkDir) {
		t.Fatalf("invocation dir=%q, want %q", inv.Dir, ws.RunWorkDir)
	}
}
