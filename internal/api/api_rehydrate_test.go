package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestAPI_Rehydrate_RequiresWorkspaceNotActive(t *testing.T) {
	t.Skip("legacy: run workspace prerequisite removed; rehydrate no longer depends on workspace status")
}

func TestAPI_Rehydrate_CreatesNewRunWithExtractedContext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
	}

	baseDir := t.TempDir()
	now := time.Now().UTC()
	exitCode := 0

	first, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    baseDir,
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, first.ID, tasks.LogAssistant, "done A")
	if err := taskStore.FinishTask(ctx, first.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  first.SessionID,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}

	second, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    baseDir,
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, second.ID, tasks.LogAssistant, "done B")
	if err := taskStore.FinishTask(ctx, second.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  second.SessionID,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish second: %v", err)
	}

	// Simulate a rehydrated session with a new provider session_id but the same conversation.
	third, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: first.ConversationID,
		Prompt:         "rehydrate: continue",
		WorkDir:        baseDir,
		SessionID:      "sess-2",
	})
	if err != nil {
		t.Fatalf("create third: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, third.ID, tasks.LogAssistant, "done C")
	if err := taskStore.FinishTask(ctx, third.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  third.SessionID,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish third: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/tasks/"+third.ID+"/rehydrate", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post rehydrate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d, want 200; body=%s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	bodyOut := decodeMutationResponse(t, res)
	requireMutationAction(t, bodyOut, "task.rehydrate")
	created := requireMutationTask(t, bodyOut)
	if created.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q, want %q", created.Mode, tasks.ModeNew)
	}
	if strings.TrimSpace(created.ConversationID) != strings.TrimSpace(first.ConversationID) {
		t.Fatalf("conversation_id=%q, want %q", created.ConversationID, first.ConversationID)
	}
	if strings.TrimSpace(created.SessionID) != "" {
		t.Fatalf("session_id=%q, want empty for new session", created.SessionID)
	}
	if !strings.Contains(created.Prompt, "do A") || !strings.Contains(created.Prompt, "done A") {
		t.Fatalf("prompt missing first run context: %q", created.Prompt)
	}
	if !strings.Contains(created.Prompt, "done B") {
		t.Fatalf("prompt missing second run context: %q", created.Prompt)
	}
	if !strings.Contains(created.Prompt, "done C") {
		t.Fatalf("prompt missing third run context: %q", created.Prompt)
	}

	logs, err := taskStore.ListLogs(ctx, created.ID, 0, 200)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream == tasks.LogSystem && strings.Contains(strings.ToLower(l.Message), "rehydrate") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rehydrate system log; logs=%v", logs)
	}
}
