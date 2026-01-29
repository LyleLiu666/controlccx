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

func TestObserver_LengthQuery_FromPromptRequirement(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	obs := &Service{Store: store}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一个10字脱口秀",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, strings.Repeat("哈", 10))

	reply, err := obs.Respond(ctx, "刚刚那个写脱口秀的任务 结果够不够字数")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, task.ID[:8]) {
		t.Fatalf("expected task id prefix in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, ">= 10") {
		t.Fatalf("expected requirement in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, "够") {
		t.Fatalf("expected '够' conclusion in reply: %q", reply.Message)
	}
}

func TestObserver_LengthQuery_NoRequirement(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	obs := &Service{Store: store}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一段脱口秀",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, "你好世界")

	reply, err := obs.Respond(ctx, "刚刚那个写脱口秀的任务 结果够不够字数")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, task.ID[:8]) {
		t.Fatalf("expected task id prefix in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, "至少") {
		t.Fatalf("expected asking for threshold in reply: %q", reply.Message)
	}
}

type stubRunner struct {
	started []string
}

func (r *stubRunner) Start(ctx context.Context, taskID string) error {
	r.started = append(r.started, taskID)
	return nil
}

func (r *stubRunner) Cancel(taskID string) bool {
	return false
}

func TestObserver_ResumeFallback_ContinueResumesLatestInterrupted(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	runner := &stubRunner{}
	obs := &Service{Store: store, Runner: runner}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Older failed task in a different session.
	old, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "old task",
		WorkDir:    ".",
		SessionID:  "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = store.FinishTask(ctx, old.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		ExitCode:   nil,
		Error:      "failed",
		SessionID:  "",
		FinishedAt: now,
	})

	// Latest interrupted task to be resumed.
	latest, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "latest task",
		WorkDir:    ".",
		SessionID:  "22222222-2222-2222-2222-222222222222",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = store.FinishTask(ctx, latest.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusInterrupted,
		ExitCode:   nil,
		Error:      "interrupted",
		SessionID:  "",
		FinishedAt: now.Add(time.Second),
	})

	reply, err := obs.Respond(ctx, "继续")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}

	// Should have started exactly one new resume run.
	if len(runner.started) != 1 {
		t.Fatalf("expected 1 started task, got %d", len(runner.started))
	}

	all, err := store.ListTasks(ctx, 50)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks after resume, got %d", len(all))
	}

	found := false
	for _, tsk := range all {
		if tsk.ID != runner.started[0] {
			continue
		}
		found = true
		if tsk.Mode != tasks.ModeResume {
			t.Fatalf("expected resume mode, got %q", tsk.Mode)
		}
		if strings.TrimSpace(tsk.SessionID) != "22222222-2222-2222-2222-222222222222" {
			t.Fatalf("expected session id to match latest interrupted session, got %q", tsk.SessionID)
		}
	}
	if !found {
		t.Fatalf("started task not found in store: %s", runner.started[0])
	}

	if !strings.Contains(reply.Message, runner.started[0][:8]) {
		t.Fatalf("expected reply to mention new run id prefix, got: %q", reply.Message)
	}
}

func TestObserver_ResumeFallback_ExplicitPrefixResumesTarget(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	runner := &stubRunner{}
	obs := &Service{Store: store, Runner: runner}

	a, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "task A",
		WorkDir:    ".",
		SessionID:  "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = store.FinishTask(ctx, a.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusInterrupted,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	})

	b, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "task B",
		WorkDir:    ".",
		SessionID:  "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = store.FinishTask(ctx, b.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	})

	reply, err := obs.Respond(ctx, "继续 "+a.ID[:8])
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if len(runner.started) != 1 {
		t.Fatalf("expected 1 started task, got %d", len(runner.started))
	}
	startedID := runner.started[0]
	started, err := store.GetTask(ctx, startedID)
	if err != nil {
		t.Fatalf("get started task: %v", err)
	}
	if strings.TrimSpace(started.SessionID) != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("expected resumed session to match explicit target, got %q", started.SessionID)
	}
	if !strings.Contains(reply.Message, startedID[:8]) {
		t.Fatalf("expected reply to mention new run id prefix, got: %q", reply.Message)
	}
}
