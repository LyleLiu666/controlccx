package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestAPI_TaskMutationEndpoints_InvalidJSON_ReturnsProblemEnvelope(t *testing.T) {
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
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	sessionKey := tasks.SessionKeyForTask(task)
	tests := []struct {
		name string
		url  string
	}{
		{name: "task.create", url: srv.URL + "/api/tasks"},
		{name: "task.resume", url: srv.URL + "/api/tasks/" + task.ID + "/resume"},
		{name: "task.rehydrate", url: srv.URL + "/api/tasks/" + task.ID + "/rehydrate"},
		{name: "task.enter_unsafe", url: srv.URL + "/api/tasks/" + task.ID + "/enter-unsafe"},
		{name: "session.continue", url: srv.URL + "/api/sessions/" + url.PathEscape(sessionKey) + "/continue"},
		{name: "session.preempt_continue", url: srv.URL + "/api/sessions/" + url.PathEscape(sessionKey) + "/preempt-continue"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := http.Post(tc.url, "application/json", bytes.NewBufferString("{"))
			if err != nil {
				t.Fatalf("post %s: %v", tc.name, err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d want %d", res.StatusCode, http.StatusBadRequest)
			}
			out := decodeMutationResponse(t, res)
			requireMutationProblemCode(t, out, "invalid_argument")
			if out.Message == "" {
				t.Fatalf("expected non-empty message")
			}
		})
	}
}
