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

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

func TestAPI_Rehydrate_RequiresWorkspaceNotActive(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:    taskStore,
		Workers:  nil,
		Observer: &observer.Service{Store: taskStore, Chat: chatStore},
		Chat:     chatStore,
		Hub:      hub,
	}

	baseDir := t.TempDir()

	prev, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    baseDir,
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create prev: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, prev.ID, tasks.LogAssistant, "done A")

	resume, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    baseDir,
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create resume: %v", err)
	}
	_, _ = taskStore.AppendLog(ctx, resume.ID, tasks.LogAssistant, "done B")

	// Active workspace must block rehydrate.
	ws := tasks.SessionWorkspace{
		Key:         tasks.SessionKey("", "sess-1"),
		WorkspaceID: "ws-1",
		Kind:        tasks.WorkspaceKindCopy,
		BaseWorkDir: baseDir,
		RunRoot:     filepath.Join(baseDir, ".ccx", "workspaces", "ws-1"),
		RunWorkDir:  filepath.Join(baseDir, ".ccx", "workspaces", "ws-1"),
		Status:      tasks.WorkspaceStatusActive,
		CreatedAt:   time.Now().UTC(),
	}
	if _, err := taskStore.UpsertSessionWorkspace(ctx, ws); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/tasks/"+resume.ID+"/rehydrate", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post rehydrate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d, want %d; body=%s", res.StatusCode, http.StatusConflict, strings.TrimSpace(string(b)))
	}

	list, err := taskStore.ListTasksWithOptions(ctx, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("tasks=%d, want 2", len(list))
	}
}

func TestAPI_Rehydrate_CreatesNewRunWithExtractedContext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:    taskStore,
		Workers:  nil,
		Observer: &observer.Service{Store: taskStore, Chat: chatStore},
		Chat:     chatStore,
		Hub:      hub,
	}

	baseDir := t.TempDir()

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

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/tasks/"+second.ID+"/rehydrate", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post rehydrate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d, want 200; body=%s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	var created tasks.Task
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q, want %q", created.Mode, tasks.ModeNew)
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
