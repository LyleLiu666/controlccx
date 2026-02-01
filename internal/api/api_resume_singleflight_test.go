package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

func TestAPI_ResumeTask_SingleFlightPerSession(t *testing.T) {
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

	prev, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create prev: %v", err)
	}

	running, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create running: %v", err)
	}
	if err := taskStore.SetRunning(ctx, running.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	payload := map[string]any{"prompt": "continue"}
	buf, _ := json.Marshal(payload)
	res, err := http.Post(srv.URL+"/api/tasks/"+prev.ID+"/resume", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post resume: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want %d", res.StatusCode, http.StatusConflict)
	}

	list, err := taskStore.ListTasksWithOptions(ctx, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("tasks=%d, want 2", len(list))
	}
}
