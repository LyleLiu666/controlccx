package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

type recordingRunnerForCancel struct {
	mu      sync.Mutex
	started []string
}

func (r *recordingRunnerForCancel) Start(ctx context.Context, taskID string) error {
	r.mu.Lock()
	r.started = append(r.started, strings.TrimSpace(taskID))
	r.mu.Unlock()
	return nil
}

func (r *recordingRunnerForCancel) Cancel(ctx context.Context, taskID string) (bool, error) {
	return false, nil
}

func TestAPI_TaskCancel_QueuedPromotesWaitingAndStartsNext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	hub := events.NewHub()
	runner := &recordingRunnerForCancel{}

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: runner,
		Hub:     hub,
	}

	first, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo 1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	waiting, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "wait",
		Prompt:          "echo 2",
		WorkDir:         ".",
	})
	if err != nil {
		t.Fatalf("create waiting: %v", err)
	}
	if waiting.Status != tasks.StatusWaiting {
		t.Fatalf("waiting status=%q want %q", waiting.Status, tasks.StatusWaiting)
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks/"+url.PathEscape(first.ID)+"/cancel", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", res.StatusCode, http.StatusOK)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v want true", out["ok"])
	}
	if strings.TrimSpace(anyString(out["promoted_task_id"])) != waiting.ID {
		t.Fatalf("promoted_task_id=%v want %v", out["promoted_task_id"], waiting.ID)
	}
	if strings.TrimSpace(anyString(out["started_task_id"])) != waiting.ID {
		t.Fatalf("started_task_id=%v want %v", out["started_task_id"], waiting.ID)
	}

	updatedFirst, err := taskStore.GetTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if updatedFirst.Status != tasks.StatusCanceled {
		t.Fatalf("first status=%q want %q", updatedFirst.Status, tasks.StatusCanceled)
	}

	updatedWaiting, err := taskStore.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatalf("get waiting: %v", err)
	}
	if updatedWaiting.Status != tasks.StatusQueued {
		t.Fatalf("waiting status=%q want %q", updatedWaiting.Status, tasks.StatusQueued)
	}

	runner.mu.Lock()
	started := append([]string{}, runner.started...)
	runner.mu.Unlock()
	if len(started) != 1 || started[0] != waiting.ID {
		t.Fatalf("started=%v want [%s]", started, waiting.ID)
	}
}

