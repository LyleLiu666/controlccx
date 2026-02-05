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

func TestObserverRespond_DeliveryForemanRequiresLLM_NoDeterministicFallback(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一篇不少于30000字、包含14个部分的文章，适合公众号",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, "# Title\n## A\n## B\nbody")
	if err := store.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	svc := &Service{Store: store}
	msg := strings.Join([]string{
		"【Delivery Foreman / 交付前哨】",
		"run_id: " + task.ID,
		"session_id: sess-1",
		"worker: claude-code",
		"status: succeeded",
		"workdir: .",
		"runs_in_session: 1",
		"",
		"prompt:",
		task.Prompt,
	}, "\n")

	reply, err := svc.Respond(ctx, msg)
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, "ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("reply=%q, want config hint mentioning ANTHROPIC_AUTH_TOKEN", reply.Message)
	}

	_, ok, err := store.GetAcceptanceState(ctx, tasks.SessionKeyForTask(task))
	if err != nil {
		t.Fatalf("get acceptance: %v", err)
	}
	if ok {
		t.Fatalf("expected no acceptance state without LLM (no deterministic fallback)")
	}
}
