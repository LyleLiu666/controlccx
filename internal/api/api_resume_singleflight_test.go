package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/events"
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
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
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
	now := time.Now().UTC()
	exitCode := 0
	if err := taskStore.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  prev.SessionID,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
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
	out := decodeMutationResponse(t, res)
	requireMutationProblemCode(t, out, "session_task_in_flight")
	if got := anyString(out.Details["existing_task_id"]); got != running.ID {
		t.Fatalf("existing_task_id=%q, want %q", got, running.ID)
	}
	if got := anyString(out.Details["existing_status"]); got != string(tasks.StatusRunning) {
		t.Fatalf("existing_status=%q, want %q", got, tasks.StatusRunning)
	}

	list, err := taskStore.ListTasksWithOptions(ctx, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("tasks=%d, want 2", len(list))
	}
}

func TestAPI_ResumeTask_SingleFlightPerSession_AwaitingApproval(t *testing.T) {
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
	now := time.Now().UTC()
	exitCode := 0
	if err := taskStore.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exitCode,
		Error:      "",
		SessionID:  prev.SessionID,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	inFlight, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "continue",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create inflight: %v", err)
	}
	if err := taskStore.SetRunning(ctx, inFlight.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := taskStore.SetAwaitingApproval(ctx, inFlight.ID); err != nil {
		t.Fatalf("set awaiting approval: %v", err)
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
	out := decodeMutationResponse(t, res)
	requireMutationProblemCode(t, out, "session_task_in_flight")
	if got := anyString(out.Details["existing_task_id"]); got != inFlight.ID {
		t.Fatalf("existing_task_id=%q, want %q", got, inFlight.ID)
	}
	if got := anyString(out.Details["existing_status"]); got != string(tasks.StatusAwaitingApproval) {
		t.Fatalf("existing_status=%q, want %q", got, tasks.StatusAwaitingApproval)
	}

	list, err := taskStore.ListTasksWithOptions(ctx, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("tasks=%d, want 2", len(list))
	}
}
