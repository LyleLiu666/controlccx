package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

func TestAPI_ListTasks_SortsByLatestExecutionTimeDesc(t *testing.T) {
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
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	older, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "older",
		WorkDir:    filepath.Join(t.TempDir(), "older"),
	})
	if err != nil {
		t.Fatalf("create older: %v", err)
	}

	newer, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "newer",
		WorkDir:    filepath.Join(t.TempDir(), "newer"),
	})
	if err != nil {
		t.Fatalf("create newer: %v", err)
	}

	if err := taskStore.SetRunning(ctx, older.ID); err != nil {
		t.Fatalf("set running older: %v", err)
	}
	finishAt := time.Now().UTC().Add(1 * time.Hour)
	if err := taskStore.FinishTask(ctx, older.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		FinishedAt: finishAt,
	}); err != nil {
		t.Fatalf("finish older: %v", err)
	}

	res, err := http.Get(srv.URL + "/api/tasks?limit=2&include_deleted=1")
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var out struct {
		Tasks []tasks.Task `json:"tasks"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(out.Tasks) != 2 {
		t.Fatalf("tasks=%d, want 2", len(out.Tasks))
	}
	if out.Tasks[0].ID != older.ID {
		t.Fatalf("first id=%q, want older=%q", out.Tasks[0].ID, older.ID)
	}
	if out.Tasks[1].ID != newer.ID {
		t.Fatalf("second id=%q, want newer=%q", out.Tasks[1].ID, newer.ID)
	}
}
