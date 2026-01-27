package worker

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_EmitsStartAndFinishLogs(t *testing.T) {
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

	m := &Manager{
		cfg:   config.Default(),
		store: store,
	}

	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}

	var start, finish string
	for _, e := range logs {
		if e.Stream != tasks.LogSystem {
			continue
		}
		if strings.HasPrefix(e.Message, "run.start ") {
			start = e.Message
		}
		if strings.HasPrefix(e.Message, "run.finish ") {
			finish = e.Message
		}
	}
	if start == "" {
		t.Fatalf("expected run.start log, logs=%v", logs)
	}
	if !strings.Contains(start, "worker=exec") {
		t.Fatalf("start=%q, want worker=exec", start)
	}
	wantCmd := `cmd="sh"`
	if runtime.GOOS == "windows" {
		wantCmd = `cmd="cmd.exe"`
	}
	if !strings.Contains(start, wantCmd) {
		t.Fatalf("start=%q, want contains %s", start, wantCmd)
	}

	if finish == "" {
		t.Fatalf("expected run.finish log, logs=%v", logs)
	}
	if !strings.Contains(finish, "status=succeeded") {
		t.Fatalf("finish=%q, want status=succeeded", finish)
	}
	if !strings.Contains(finish, "exit_code=0") {
		t.Fatalf("finish=%q, want exit_code=0", finish)
	}
}
