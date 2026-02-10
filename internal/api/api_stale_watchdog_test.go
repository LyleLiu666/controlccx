package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestAPI_RunStaleWatchdogOnce_InterruptsStaleInFlightTask(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo stale",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	old := time.Now().UTC().Add(-16 * time.Minute).UnixMilli()
	if _, err := conn.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, old, task.ID); err != nil {
		t.Fatalf("set old updated_at: %v", err)
	}

	n, err := apiSvc.runStaleWatchdogOnce(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("run watchdog: %v", err)
	}
	if n != 1 {
		t.Fatalf("updated=%d, want 1", n)
	}

	updated, err := taskStore.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusInterrupted {
		t.Fatalf("status=%q, want %q", updated.Status, tasks.StatusInterrupted)
	}

	logs, err := taskStore.ListLogs(ctx, task.ID, 0, 200)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, e := range logs {
		if strings.Contains(strings.ToLower(e.Message), "stale watchdog") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected stale watchdog system log")
	}
}

func TestAPI_RunStaleWatchdogOnce_SkipsFreshHeartbeatTask(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo fresh",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	old := time.Now().UTC().Add(-16 * time.Minute).UnixMilli()
	if _, err := conn.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, old, task.ID); err != nil {
		t.Fatalf("set old updated_at: %v", err)
	}
	if err := taskStore.TouchTask(ctx, task.ID); err != nil {
		t.Fatalf("touch task: %v", err)
	}

	n, err := apiSvc.runStaleWatchdogOnce(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("run watchdog: %v", err)
	}
	if n != 0 {
		t.Fatalf("updated=%d, want 0", n)
	}

	updated, err := taskStore.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status=%q, want %q", updated.Status, tasks.StatusRunning)
	}
}

type staleWatchdogRunner struct {
	cancelCalls []string
	cancelOK    bool
	cancelErr   error
}

func (r *staleWatchdogRunner) Start(ctx context.Context, taskID string) error {
	return nil
}

func (r *staleWatchdogRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	r.cancelCalls = append(r.cancelCalls, taskID)
	return r.cancelOK, r.cancelErr
}

func TestAPI_RunStaleWatchdogOnce_AttemptsRunnerCancel(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	runner := &staleWatchdogRunner{cancelOK: true}
	apiSvc := &API{Tasks: taskStore, Hub: hub, Workers: runner}

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo stale",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	now := time.Now().UTC()
	old := now.Add(-16 * time.Minute).UnixMilli()
	if _, err := conn.ExecContext(ctx, `UPDATE tasks SET updated_at = ? WHERE id = ?`, old, task.ID); err != nil {
		t.Fatalf("set old updated_at: %v", err)
	}

	n, err := apiSvc.runStaleWatchdogOnce(ctx, now)
	if err != nil {
		t.Fatalf("run watchdog: %v", err)
	}
	if n != 1 {
		t.Fatalf("updated=%d, want 1", n)
	}
	if len(runner.cancelCalls) != 1 || runner.cancelCalls[0] != task.ID {
		t.Fatalf("cancel calls=%v, want [%s]", runner.cancelCalls, task.ID)
	}
}
