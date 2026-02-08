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
	"controlccx/internal/tasks"
)

type approvalsRunner struct {
	store *tasks.Store
}

func (r approvalsRunner) Start(ctx context.Context, taskID string) error { return nil }
func (r approvalsRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	return false, nil
}

func (r approvalsRunner) SubmitApprovalDecision(ctx context.Context, taskID string, approvalID string, decision string, reason string) error {
	var status tasks.ApprovalStatus
	switch decision {
	case "approve":
		status = tasks.ApprovalStatusApproved
	case "deny":
		status = tasks.ApprovalStatusDenied
	default:
		return &tasks.ApprovalNotPendingError{ApprovalID: approvalID, Status: tasks.ApprovalStatusDenied}
	}
	return r.store.UpdateApprovalRequestDecision(ctx, approvalID, tasks.UpdateApprovalRequestDecisionInput{
		Status: status,
		Reason: reason,
	})
}

func TestAPI_TaskApprovals_ListAndDecide(t *testing.T) {
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

	apiSvc := &API{
		Tasks:   store,
		Workers: approvalsRunner{store: store},
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/tasks/" + task.ID + "/approvals?status=pending")
	if err != nil {
		t.Fatalf("get approvals: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var listResp struct {
		Approvals []tasks.ApprovalRequest `json:"approvals"`
	}
	if err := json.NewDecoder(res.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listResp.Approvals) != 1 || listResp.Approvals[0].ID != ar.ID {
		t.Fatalf("approvals=%+v want id=%q", listResp.Approvals, ar.ID)
	}

	body, _ := json.Marshal(map[string]any{"decision": "approve", "reason": "ok"})
	decRes, err := http.Post(srv.URL+"/api/tasks/"+task.ID+"/approvals/"+ar.ID+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post decision: %v", err)
	}
	defer decRes.Body.Close()
	if decRes.StatusCode != http.StatusOK {
		t.Fatalf("decision status=%d want 200", decRes.StatusCode)
	}

	updated, ok, err := store.GetApprovalRequest(ctx, ar.ID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	if updated.Status != tasks.ApprovalStatusApproved || updated.Reason != "ok" {
		t.Fatalf("updated=%+v want status=%q reason=%q", updated, tasks.ApprovalStatusApproved, "ok")
	}

	decRes2, err := http.Post(srv.URL+"/api/tasks/"+task.ID+"/approvals/"+ar.ID+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post decision 2: %v", err)
	}
	defer decRes2.Body.Close()
	if decRes2.StatusCode != http.StatusConflict {
		t.Fatalf("decision2 status=%d want 409", decRes2.StatusCode)
	}
}
