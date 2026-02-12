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

type failingRunner struct {
	startErr  error
	cancelOK  bool
	cancelErr error
}

func (r failingRunner) Start(ctx context.Context, taskID string) error {
	return r.startErr
}

func (r failingRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	return r.cancelOK, r.cancelErr
}

func TestAPI_TasksCreate_DegradesExplicitlyWhenRunnerUnavailable(t *testing.T) {
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
		Workers: failingRunner{startErr: context.DeadlineExceeded},
		Hub:     hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    t.TempDir(),
	}
	buf, _ := json.Marshal(body)
	res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", res.StatusCode, http.StatusServiceUnavailable)
	}

	out := decodeMutationResponse(t, res)
	requireMutationProblemCode(t, out, "runner_unavailable")
	taskID := anyString(out.Details["task_id"])
	if strings.TrimSpace(taskID) == "" {
		t.Fatalf("expected task_id in response details: %+v", out)
	}
	if !strings.Contains(out.Hint, "runner") {
		t.Fatalf("expected hint mentioning runner, got: %q", out.Hint)
	}

	task, err := taskStore.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != tasks.StatusFailed {
		t.Fatalf("status=%q want %q", task.Status, tasks.StatusFailed)
	}
}

func TestAPI_SessionContinue_DegradesExplicitlyWhenRunnerUnavailable(t *testing.T) {
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
		Workers: failingRunner{startErr: context.DeadlineExceeded},
		Hub:     hub,
	}

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

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", res.StatusCode, http.StatusServiceUnavailable)
	}
	out := decodeMutationResponse(t, res)
	requireMutationProblemCode(t, out, "runner_unavailable")
	taskID := anyString(out.Details["task_id"])
	if strings.TrimSpace(taskID) == "" {
		t.Fatalf("expected task_id in response details: %+v", out)
	}
}

func TestAPI_TaskCancel_DegradesExplicitlyWhenRunnerUnavailable(t *testing.T) {
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
		Workers: failingRunner{cancelErr: context.DeadlineExceeded},
		Hub:     hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetRunning(ctx, task.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}
	taskID := task.ID
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks/"+url.PathEscape(taskID)+"/cancel", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d", res.StatusCode, http.StatusServiceUnavailable)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["error"] != "runner_unavailable" {
		t.Fatalf("error=%v want %v", out["error"], "runner_unavailable")
	}
}
