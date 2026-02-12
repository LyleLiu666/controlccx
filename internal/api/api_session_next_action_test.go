package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

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

func TestAPI_SessionNextActionExecute_ContinuePathUsesUnifiedEnvelope(t *testing.T) {
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
		SessionID:  "sess-exec",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	body, _ := json.Marshal(map[string]any{"prompt": "continue"})
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/next-action/execute", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post next action execute: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	out := decodeMutationResponse(t, res)
	requireMutationAction(t, out, "session.next_action_execute")
	next := requireMutationTask(t, out)
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q, want %q", next.Mode, tasks.ModeResume)
	}
	if next.ConversationID != task.ConversationID {
		t.Fatalf("conversation_id=%q, want %q", next.ConversationID, task.ConversationID)
	}
}

func TestAPI_SessionNextActionExecute_UnsupportedActionReturnsProblem(t *testing.T) {
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
		SessionID:  "sess-approval",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.SetAwaitingApproval(ctx, task.ID); err != nil {
		t.Fatalf("set awaiting approval: %v", err)
	}
	if _, err := taskStore.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskHigh,
		Summary:    "needs approval",
		Raw:        []byte(`{"cmd":"rm -rf"}`),
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	key := tasks.SessionKeyForTask(task)
	res, err := http.Post(srv.URL+"/api/sessions/"+url.PathEscape(key)+"/next-action/execute", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("post next action execute: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", res.StatusCode)
	}
	out := decodeMutationResponse(t, res)
	requireMutationProblemCode(t, out, "unsupported")
}
