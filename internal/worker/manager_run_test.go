package worker

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

// TestManager_Run_Exec_HappyPath verifies the simplest case:
// a WorkerExec task runs echo, transitions to succeeded.
func TestManager_Run_Exec_HappyPath(t *testing.T) {
	store := newMockStore()
	hub := &mockHub{}

	task := tasks.Task{
		ID:         "task-exec-happy",
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    t.TempDir(),
		Status:     tasks.StatusQueued,
	}
	store.mu.Lock()
	store.tasks[task.ID] = task
	store.taskStatus[task.ID] = task.Status
	store.mu.Unlock()

	runner := newMockProcessRunner()

	m := &Manager{
		cfg:             config.Default(),
		store:           store,
		hub:             hub,
		runner:          runner,
		cancels:         make(map[string]context.CancelFunc),
		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		runner.mu.Lock()
		if len(runner.runs) > 0 {
			runner.runs[0].emulateFinish(0, nil)
		}
		runner.mu.Unlock()
	}()

	err := m.run(context.Background(), task)
	if err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	calls := store.getFinishCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one FinishTask call")
	}
	last := calls[len(calls)-1]
	if last.Input.Status != tasks.StatusSucceeded {
		t.Fatalf("FinishTask status=%q, want %q", last.Input.Status, tasks.StatusSucceeded)
	}
}

// TestManager_Run_Exec_CommandNotFound verifies that a missing executable
// transitions the task to failed with a meaningful error.
func TestManager_Run_Exec_CommandNotFound(t *testing.T) {
	store := newMockStore()
	hub := &mockHub{}

	task := tasks.Task{
		ID:         "task-exec-notfound",
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "/nonexistent-binary-12345",
		WorkDir:    t.TempDir(),
		Status:     tasks.StatusQueued,
	}
	store.mu.Lock()
	store.tasks[task.ID] = task
	store.taskStatus[task.ID] = task.Status
	store.mu.Unlock()

	runner := newMockProcessRunner()
	runner.spawnErr = exec.ErrNotFound

	m := &Manager{
		cfg:             config.Default(),
		store:           store,
		hub:             hub,
		runner:          runner,
		cancels:         make(map[string]context.CancelFunc),
		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}

	// run() calls failTask() internally, which writes to DB and returns the error.
	// For exec worker, the shell itself might succeed (sh -lc "nonexistent") so
	// we just check the task was finished with a non-succeeded status.
	_ = m.run(context.Background(), task)

	calls := store.getFinishCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one FinishTask call")
	}
	last := calls[len(calls)-1]
	if last.Input.Status != tasks.StatusFailed {
		t.Fatalf("FinishTask status=%q, want %q", last.Input.Status, tasks.StatusFailed)
	}
}

// TestManager_Run_Exec_ContextCancel verifies that canceling the context
// transitions the task to canceled.
func TestManager_Run_Exec_ContextCancel(t *testing.T) {
	store := newMockStore()
	hub := &mockHub{}

	task := tasks.Task{
		ID:         "task-exec-cancel",
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "sleep 60",
		WorkDir:    t.TempDir(),
		Status:     tasks.StatusQueued,
	}
	store.mu.Lock()
	store.tasks[task.ID] = task
	store.taskStatus[task.ID] = task.Status
	store.mu.Unlock()

	runner := newMockProcessRunner()

	m := &Manager{
		cfg:             config.Default(),
		store:           store,
		hub:             hub,
		runner:          runner,
		cancels:         make(map[string]context.CancelFunc),
		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- m.run(ctx, task)
	}()

	// Give the process time to start, then cancel
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("run() returned (expected) error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: run() did not exit after context cancel")
	}

	calls := store.getFinishCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one FinishTask call")
	}
	last := calls[len(calls)-1]
	if last.Input.Status != tasks.StatusCanceled {
		t.Fatalf("FinishTask status=%q, want %q", last.Input.Status, tasks.StatusCanceled)
	}
}

// TestManager_Run_Exec_NonZeroExit verifies that a command exiting with
// non-zero transitions the task to failed.
func TestManager_Run_Exec_NonZeroExit(t *testing.T) {
	store := newMockStore()
	hub := &mockHub{}

	task := tasks.Task{
		ID:         "task-exec-fail",
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "exit 42",
		WorkDir:    t.TempDir(),
		Status:     tasks.StatusQueued,
	}
	store.mu.Lock()
	store.tasks[task.ID] = task
	store.taskStatus[task.ID] = task.Status
	store.mu.Unlock()

	runner := newMockProcessRunner()

	m := &Manager{
		cfg:             config.Default(),
		store:           store,
		hub:             hub,
		runner:          runner,
		cancels:         make(map[string]context.CancelFunc),
		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		runner.mu.Lock()
		if len(runner.runs) > 0 {
			runner.runs[0].emulateFinish(42, fmt.Errorf("exit status 42"))
		}
		runner.mu.Unlock()
	}()

	err := m.run(context.Background(), task)
	if err != nil {
		t.Fatalf("run() returned error: %v (expected nil for non-zero exit)", err)
	}

	calls := store.getFinishCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one FinishTask call")
	}
	last := calls[len(calls)-1]
	if last.Input.Status != tasks.StatusFailed {
		t.Fatalf("FinishTask status=%q, want %q", last.Input.Status, tasks.StatusFailed)
	}
	if last.Input.ExitCode == nil || *last.Input.ExitCode != 42 {
		t.Fatalf("FinishTask exit_code=%v, want 42", last.Input.ExitCode)
	}
}

// TestManager_Run_MockEventsPublished verifies that task events are
// published to the hub during a run.
func TestManager_Run_MockEventsPublished(t *testing.T) {
	store := newMockStore()
	hub := &mockHub{}

	task := tasks.Task{
		ID:         "task-events",
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo events",
		WorkDir:    t.TempDir(),
		Status:     tasks.StatusQueued,
	}
	store.mu.Lock()
	store.tasks[task.ID] = task
	store.taskStatus[task.ID] = task.Status
	store.mu.Unlock()

	runner := newMockProcessRunner()

	m := &Manager{
		cfg:             config.Default(),
		store:           store,
		hub:             hub,
		runner:          runner,
		cancels:         make(map[string]context.CancelFunc),
		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		runner.mu.Lock()
		if len(runner.runs) > 0 {
			runner.runs[0].emulateFinish(0, nil)
		}
		runner.mu.Unlock()
	}()

	_ = m.run(context.Background(), task)

	hub.mu.Lock()
	count := len(hub.events)
	hub.mu.Unlock()
	if count == 0 {
		t.Fatal("expected at least one event published to hub")
	}
}
