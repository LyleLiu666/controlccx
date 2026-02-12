package taskops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

type recordingRunner struct {
	mu        sync.Mutex
	started   []string
	canceled  []string
	startErr  error
	cancelOK  bool
	cancelErr error
}

func (r *recordingRunner) Start(ctx context.Context, taskID string) error {
	r.mu.Lock()
	r.started = append(r.started, strings.TrimSpace(taskID))
	r.mu.Unlock()
	return r.startErr
}

func (r *recordingRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	r.mu.Lock()
	r.canceled = append(r.canceled, strings.TrimSpace(taskID))
	r.mu.Unlock()
	return r.cancelOK, r.cancelErr
}

func TestCancelTask_QueuedCancelsAndPromotesWaiting_StartsNext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	waiting, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}
	if waiting.Status != tasks.StatusWaiting {
		t.Fatalf("waiting status=%q want %q", waiting.Status, tasks.StatusWaiting)
	}

	runner := &recordingRunner{}
	svc := &Service{Tasks: store, Workers: runner}

	out, err := svc.CancelTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !out.Requested {
		t.Fatalf("requested=false want true")
	}
	if out.StatusBefore != tasks.StatusQueued || out.StatusAfter != tasks.StatusCanceled {
		t.Fatalf("status_before/after=%q/%q want %q/%q", out.StatusBefore, out.StatusAfter, tasks.StatusQueued, tasks.StatusCanceled)
	}
	if strings.TrimSpace(out.PromotedTaskID) != waiting.ID {
		t.Fatalf("promoted_task_id=%q want %q", out.PromotedTaskID, waiting.ID)
	}
	if strings.TrimSpace(out.StartedTaskID) != waiting.ID {
		t.Fatalf("started_task_id=%q want %q", out.StartedTaskID, waiting.ID)
	}
	if strings.TrimSpace(out.NextStartError) != "" {
		t.Fatalf("next_start_error=%q want empty", out.NextStartError)
	}

	updatedFirst, err := store.GetTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if updatedFirst.Status != tasks.StatusCanceled {
		t.Fatalf("first status=%q want %q", updatedFirst.Status, tasks.StatusCanceled)
	}

	updatedWaiting, err := store.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("get waiting: %v", err)
	}
	if updatedWaiting.Status != tasks.StatusQueued {
		t.Fatalf("waiting status=%q want %q", updatedWaiting.Status, tasks.StatusQueued)
	}

	runner.mu.Lock()
	started := append([]string{}, runner.started...)
	runner.mu.Unlock()
	if len(started) != 1 || started[0] != waiting.ID {
		t.Fatalf("started=%v want [%s]", started, waiting.ID)
	}
}

func TestCancelTask_QueuedStartFailureMarksNextFailed(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	waiting, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}

	runner := &recordingRunner{startErr: errors.New("runner down")}
	svc := &Service{Tasks: store, Workers: runner}

	out, err := svc.CancelTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if strings.TrimSpace(out.PromotedTaskID) != waiting.ID {
		t.Fatalf("promoted_task_id=%q want %q", out.PromotedTaskID, waiting.ID)
	}
	if strings.TrimSpace(out.StartedTaskID) != "" {
		t.Fatalf("started_task_id=%q want empty", out.StartedTaskID)
	}
	if strings.TrimSpace(out.NextStartError) == "" {
		t.Fatalf("expected next_start_error")
	}

	updatedWaiting, err := store.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("get waiting: %v", err)
	}
	if updatedWaiting.Status != tasks.StatusFailed {
		t.Fatalf("waiting status=%q want %q", updatedWaiting.Status, tasks.StatusFailed)
	}
	if !strings.Contains(updatedWaiting.Error, "runner down") {
		t.Fatalf("waiting error=%q want mention runner down", updatedWaiting.Error)
	}
}

func TestCancelTask_WaitingSetsCanceled(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	waiting, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}
	if waiting.Status != tasks.StatusWaiting {
		t.Fatalf("waiting status=%q want %q", waiting.Status, tasks.StatusWaiting)
	}

	runner := &recordingRunner{}
	svc := &Service{Tasks: store, Workers: runner}

	out, err := svc.CancelTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !out.Requested {
		t.Fatalf("requested=false want true")
	}
	if out.StatusBefore != tasks.StatusWaiting || out.StatusAfter != tasks.StatusCanceled {
		t.Fatalf("status_before/after=%q/%q want %q/%q", out.StatusBefore, out.StatusAfter, tasks.StatusWaiting, tasks.StatusCanceled)
	}

	updated, err := store.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("get waiting: %v", err)
	}
	if updated.Status != tasks.StatusCanceled {
		t.Fatalf("waiting status=%q want %q", updated.Status, tasks.StatusCanceled)
	}

	runner.mu.Lock()
	canceled := append([]string{}, runner.canceled...)
	runner.mu.Unlock()
	if len(canceled) != 0 {
		t.Fatalf("runner cancel calls=%v want none", canceled)
	}
	_ = first // keep first alive for workdir busy
}

func TestCancelTask_RunningCallsRunnerCancelAndLogs(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	runner := &recordingRunner{cancelOK: true}
	svc := &Service{Tasks: store, Workers: runner}

	out, err := svc.CancelTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !out.Requested || !out.RunnerCancelAttempted || !out.RunnerCancelOK {
		t.Fatalf("unexpected cancel result: %+v", out)
	}
	if out.StatusAfter != tasks.StatusRunning {
		t.Fatalf("status_after=%q want %q", out.StatusAfter, tasks.StatusRunning)
	}

	logs, err := store.ListLogsTail(ctx, task.ID, 5)
	if err != nil {
		t.Fatalf("list logs tail: %v", err)
	}
	found := false
	for _, l := range logs {
		if strings.TrimSpace(l.Message) == "cancel requested" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cancel requested log, got: %+v", logs)
	}
}

