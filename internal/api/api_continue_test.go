package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestAPI_SessionContinue_CreatesResumeRun(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	first, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := taskStore.FinishTask(ctx, first.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(first)
	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post continue: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var next tasks.Task
	if err := json.NewDecoder(res.Body).Decode(&next); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q, want %q", next.Mode, tasks.ModeResume)
	}
	if strings.TrimSpace(next.SessionID) != "sess-1" {
		t.Fatalf("session_id=%q, want %q", next.SessionID, "sess-1")
	}
	if next.ConversationID != first.ConversationID {
		t.Fatalf("conversation_id=%q, want %q", next.ConversationID, first.ConversationID)
	}
}

func TestAPI_SessionContinue_FallsBackToRehydrateOnNoConversationFound(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	apiSvc := &API{Tasks: taskStore, Hub: hub}

	first, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, first.ID, tasks.LogAssistant, "done A")
	if err := taskStore.FinishTask(ctx, first.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	resume, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeResume,
		ConversationID: first.ConversationID,
		Prompt:         "continue",
		WorkDir:        ".",
		SessionID:      "sess-1",
	})
	if err != nil {
		t.Fatalf("create resume: %v", err)
	}
	if err := taskStore.SetWarning(ctx, resume.ID, "resume failed: No conversation found with session ID: sess-1"); err != nil {
		t.Fatalf("set warning: %v", err)
	}
	if err := taskStore.FinishTask(ctx, resume.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish resume: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(first)
	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post continue: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var next tasks.Task
	if err := json.NewDecoder(res.Body).Decode(&next); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if next.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q, want %q", next.Mode, tasks.ModeNew)
	}
	if next.ConversationID != first.ConversationID {
		t.Fatalf("conversation_id=%q, want %q", next.ConversationID, first.ConversationID)
	}
	if !strings.Contains(next.Prompt, "do A") || !strings.Contains(next.Prompt, "done A") {
		t.Fatalf("rehydrate prompt missing context: %q", next.Prompt)
	}
	if !strings.Contains(next.Prompt, "[controlccx rehydrate]") {
		t.Fatalf("prompt missing header: %q", next.Prompt)
	}
}

func TestAPI_SessionContinue_QueuesWhenHasQueuedOrRunning(t *testing.T) {
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
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post continue: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", res.StatusCode)
	}
	var queued struct {
		Queued         bool   `json:"queued"`
		QueueID        string `json:"queue_id"`
		Position       int    `json:"position"`
		ExistingTaskID string `json:"existing_task_id"`
		ExistingStatus string `json:"existing_status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&queued); err != nil {
		t.Fatalf("decode queued: %v", err)
	}
	if !queued.Queued {
		t.Fatalf("queued=%v, want true", queued.Queued)
	}
	if strings.TrimSpace(queued.QueueID) == "" {
		t.Fatalf("queue_id is empty")
	}
	if queued.Position != 1 {
		t.Fatalf("position=%d, want 1", queued.Position)
	}
	if queued.ExistingTaskID != task.ID {
		t.Fatalf("existing_task_id=%q, want %q", queued.ExistingTaskID, task.ID)
	}
	if queued.ExistingStatus != string(tasks.StatusQueued) {
		t.Fatalf("existing_status=%q, want %q", queued.ExistingStatus, tasks.StatusQueued)
	}
}

func TestAPI_SessionContinue_QueuesWhenAwaitingApproval(t *testing.T) {
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
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := taskStore.SetAwaitingApproval(ctx, task.ID); err != nil {
		t.Fatalf("set awaiting approval: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post continue: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", res.StatusCode)
	}
	var queued struct {
		Queued         bool   `json:"queued"`
		QueueID        string `json:"queue_id"`
		Position       int    `json:"position"`
		ExistingTaskID string `json:"existing_task_id"`
		ExistingStatus string `json:"existing_status"`
	}
	if err := json.NewDecoder(res.Body).Decode(&queued); err != nil {
		t.Fatalf("decode queued: %v", err)
	}
	if !queued.Queued {
		t.Fatalf("queued=%v, want true", queued.Queued)
	}
	if strings.TrimSpace(queued.QueueID) == "" {
		t.Fatalf("queue_id is empty")
	}
	if queued.ExistingTaskID != task.ID {
		t.Fatalf("existing_task_id=%q, want %q", queued.ExistingTaskID, task.ID)
	}
	if queued.ExistingStatus != string(tasks.StatusAwaitingApproval) {
		t.Fatalf("existing_status=%q, want %q", queued.ExistingStatus, tasks.StatusAwaitingApproval)
	}
}

func TestAPI_SessionContinue_SupportsLegacySessionKey(t *testing.T) {
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
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := "s:sess-1"
	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/continue", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post continue: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var next tasks.Task
	if err := json.NewDecoder(res.Body).Decode(&next); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q, want %q", next.Mode, tasks.ModeResume)
	}
	if next.ConversationID != task.ConversationID {
		t.Fatalf("conversation_id=%q, want %q", next.ConversationID, task.ConversationID)
	}
}
