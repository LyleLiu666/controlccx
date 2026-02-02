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

func TestAPI_CreateTask_RejectsWhenWorkdirBusy(t *testing.T) {
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

	body := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	}
	buf, _ := json.Marshal(body)

	req1, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request1: %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "key-a")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("do1: %v", err)
	}
	res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("status1=%d, want 200", res1.StatusCode)
	}

	req2, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request2: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "key-b")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do2: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("status2=%d, want 409", res2.StatusCode)
	}
}
