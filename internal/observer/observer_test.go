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

func TestObserver_RespondWithoutLLM_FailFast_NoHeuristicAnswer(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	obs := &Service{Store: store}

	reply, err := obs.Respond(ctx, "我们有几个任务在执行？")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("expected config hint mentioning ANTHROPIC_AUTH_TOKEN, got: %q", reply.Message)
	}
}

func TestObserver_Respond_ResumeViaLLMToolCall(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	runner := &stubRunner{}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do thing",
		WorkDir:    ".",
		SessionID:  "22222222-2222-2222-2222-222222222222",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusInterrupted,
		ExitCode:   nil,
		Error:      "interrupted",
		SessionID:  task.SessionID,
		FinishedAt: now,
	})

	llm := &stubBackend{
		name: "stub",
		outputs: []string{
			`{"action":"tool","tool":"session_continue","args":{"task_id":"` + task.ID + `"}}`,
			`{"action":"final","message":"resumed"}`,
		},
	}

	obs := &Service{Store: store, Runner: runner, LLM: llm}
	reply, err := obs.Respond(ctx, "继续")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if reply.Message != "resumed" {
		t.Fatalf("reply=%q, want %q", reply.Message, "resumed")
	}
	if len(llm.prompts) == 0 {
		t.Fatalf("expected LLM backend to be called (no heuristic resume path)")
	}
	if len(runner.started) != 1 {
		t.Fatalf("expected 1 started task, got %d", len(runner.started))
	}

	startedID := runner.started[0]
	started, err := store.GetTask(ctx, startedID)
	if err != nil {
		t.Fatalf("get started task: %v", err)
	}
	if started.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q, want %q", started.Mode, tasks.ModeResume)
	}
	if strings.TrimSpace(started.SessionID) != strings.TrimSpace(task.SessionID) {
		t.Fatalf("session_id=%q, want %q", started.SessionID, task.SessionID)
	}
	if strings.TrimSpace(started.ConversationID) != strings.TrimSpace(task.ConversationID) {
		t.Fatalf("conversation_id=%q, want %q", started.ConversationID, task.ConversationID)
	}
}
