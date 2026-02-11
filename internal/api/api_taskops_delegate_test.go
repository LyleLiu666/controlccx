package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

func TestAPI_TaskResume_UsesTaskOpsWhenConfigured(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "first",
		WorkDir:    t.TempDir(),
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.SetRunning(ctx, prev.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status: tasks.StatusSucceeded,
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	apiSvc := &API{
		Tasks:   store,
		TaskOps: &taskops.Service{Tasks: store},
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	payload, _ := json.Marshal(map[string]any{"prompt": "continue"})
	res, err := http.Post(srv.URL+"/api/tasks/"+prev.ID+"/resume", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post resume: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var out tasks.Task
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q want %q", out.Mode, tasks.ModeResume)
	}
	if out.SessionID != "sess-1" {
		t.Fatalf("session_id=%q want %q", out.SessionID, "sess-1")
	}
}

func TestAPI_TaskApprovals_Decision_WithTaskOps(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ar, err := store.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "WebSearch",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "lookup weather",
		Raw:        []byte(`{"q":"weather"}`),
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	runner := approvalsRunner{store: store}
	ops := &taskops.Service{
		Tasks:   store,
		Workers: runner,
	}
	apiSvc := &API{
		Tasks:   store,
		Workers: runner,
		TaskOps: ops,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{"decision": "approve", "reason": "ok"})
	decRes, err := http.Post(srv.URL+"/api/tasks/"+task.ID+"/approvals/"+ar.ID+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post decision: %v", err)
	}
	defer decRes.Body.Close()
	if decRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", decRes.StatusCode)
	}

	decRes2, err := http.Post(srv.URL+"/api/tasks/"+task.ID+"/approvals/"+ar.ID+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post decision 2: %v", err)
	}
	defer decRes2.Body.Close()
	if decRes2.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d want 409", decRes2.StatusCode)
	}
	var conflict struct {
		Error      string `json:"error"`
		ApprovalID string `json:"approval_id"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(decRes2.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode conflict: %v", err)
	}
	if conflict.Error != "approval_not_pending" {
		t.Fatalf("error=%q want %q", conflict.Error, "approval_not_pending")
	}
	if conflict.ApprovalID != ar.ID {
		t.Fatalf("approval_id=%q want %q", conflict.ApprovalID, ar.ID)
	}
	if conflict.Status != string(tasks.ApprovalStatusApproved) {
		t.Fatalf("status=%q want %q", conflict.Status, tasks.ApprovalStatusApproved)
	}
}

func TestAPI_TaskApprovals_Decision_TaskOpsRunnerUnsupportedReturns503(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ar, err := store.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "WebSearch",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "lookup weather",
		Raw:        []byte(`{"q":"weather"}`),
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	runner := noopRunner{}
	ops := &taskops.Service{
		Tasks:   store,
		Workers: runner,
	}
	apiSvc := &API{
		Tasks:   store,
		Workers: runner,
		TaskOps: ops,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{"decision": "approve", "reason": "ok"})
	decRes, err := http.Post(srv.URL+"/api/tasks/"+task.ID+"/approvals/"+ar.ID+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post decision: %v", err)
	}
	defer decRes.Body.Close()
	if decRes.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", decRes.StatusCode)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(decRes.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errBody.Error != "runner_unavailable" {
		t.Fatalf("error=%q want %q", errBody.Error, "runner_unavailable")
	}
}
