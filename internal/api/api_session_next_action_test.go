package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_SessionNextAction_ReturnsActionAndReason(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    filepath.Join(t.TempDir(), "proj"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetAwaitingApproval(ctx, task.ID); err != nil {
		t.Fatalf("set awaiting approval: %v", err)
	}
	ar, err := taskStore.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskHigh,
		Summary:    "needs approval",
		Raw:        []byte(`{"cmd":"rm -rf"}`),
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	res, err := http.Get(srv.URL + "/api/sessions/" + url.PathEscape(key) + "/next-action")
	if err != nil {
		t.Fatalf("get next action: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	var out tasks.NextAction
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Action != tasks.NextActionResolveApproval {
		t.Fatalf("action=%q, want %q", out.Action, tasks.NextActionResolveApproval)
	}
	if out.Reason != "pending_approval" {
		t.Fatalf("reason=%q, want %q", out.Reason, "pending_approval")
	}
	if out.TaskID != task.ID {
		t.Fatalf("task_id=%q, want %q", out.TaskID, task.ID)
	}
	if out.ApprovalID != ar.ID {
		t.Fatalf("approval_id=%q, want %q", out.ApprovalID, ar.ID)
	}
}

func TestAPI_SessionNextAction_MethodNotAllowed(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    filepath.Join(t.TempDir(), "proj"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/sessions/"+url.PathEscape(key)+"/next-action", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post next action: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", res.StatusCode)
	}
}
