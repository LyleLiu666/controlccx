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

type fakeRunner struct {
	started []string
	canceled []string
	startErr error
	cancelOK bool
}

func (r *fakeRunner) Start(ctx context.Context, taskID string) error {
	r.started = append(r.started, taskID)
	return r.startErr
}

func (r *fakeRunner) Cancel(taskID string) bool {
	r.canceled = append(r.canceled, taskID)
	return r.cancelOK
}

func TestTools_taskResume_CreatesAndStartsResumeRun(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	res, err := tools["task_resume"].Run(ctx, map[string]any{
		"task_id": prev.ID,
		"prompt":  "继续",
	})
	if err != nil {
		t.Fatalf("tool run: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	if len(r.started) != 1 {
		t.Fatalf("started=%v, want 1 start", r.started)
	}

	next, err := store.GetTask(ctx, r.started[0])
	if err != nil {
		t.Fatalf("get resumed task: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q want %q", next.Mode, tasks.ModeResume)
	}
	if strings.TrimSpace(next.SessionID) != "sess-1" {
		t.Fatalf("session_id=%q want %q", next.SessionID, "sess-1")
	}
	if next.Prompt != "继续" {
		t.Fatalf("prompt=%q want %q", next.Prompt, "继续")
	}
	if next.WorkDir != "." {
		t.Fatalf("workdir=%q want %q", next.WorkDir, ".")
	}
}

func TestTools_taskResume_RejectsOverlappingRunsInSession(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	running, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "running",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create running: %v", err)
	}
	if err := store.SetRunning(ctx, running.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	_, err = tools["task_resume"].Run(ctx, map[string]any{
		"task_id": prev.ID,
		"prompt":  "continue",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already has a running task") {
		t.Fatalf("err=%q, want contains %q", err.Error(), "already has a running task")
	}
	if len(r.started) != 0 {
		t.Fatalf("started=%v, want 0", r.started)
	}
}
