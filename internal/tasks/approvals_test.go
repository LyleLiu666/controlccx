package tasks

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestApprovalRequests_CRUD(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerClaudeCode,
		Mode:       ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	ar, err := store.CreateApprovalRequest(ctx, CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  RiskHigh,
		Summary:    "run command",
		Raw:        []byte(`{"tool":"bash","command":"echo hi"}`),
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}
	if ar.ID == "" {
		t.Fatalf("expected approval id")
	}
	if ar.Status != ApprovalStatusPending {
		t.Fatalf("status=%q, want %q", ar.Status, ApprovalStatusPending)
	}

	list, err := store.ListApprovalRequestsByTask(ctx, task.ID, ListApprovalRequestsOptions{})
	if err != nil {
		t.Fatalf("ListApprovalRequestsByTask: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len=%d, want 1", len(list))
	}

	if err := store.UpdateApprovalRequestDecision(ctx, ar.ID, UpdateApprovalRequestDecisionInput{
		Status: ApprovalStatusApproved,
		Reason: "ok",
	}); err != nil {
		t.Fatalf("UpdateApprovalRequestDecision: %v", err)
	}

	if err := store.UpdateApprovalRequestDecision(ctx, ar.ID, UpdateApprovalRequestDecisionInput{
		Status: ApprovalStatusDenied,
		Reason: "nope",
	}); err == nil {
		t.Fatalf("expected second decision to fail")
	} else {
		var notPending *ApprovalNotPendingError
		if !errors.As(err, &notPending) {
			t.Fatalf("second decision err=%T, want *ApprovalNotPendingError", err)
		}
	}

	approved, err := store.ListApprovalRequestsByTask(ctx, task.ID, ListApprovalRequestsOptions{Status: ApprovalStatusApproved})
	if err != nil {
		t.Fatalf("ListApprovalRequestsByTask approved: %v", err)
	}
	if len(approved) != 1 {
		t.Fatalf("approved len=%d, want 1", len(approved))
	}
	if approved[0].Status != ApprovalStatusApproved || approved[0].Reason != "ok" {
		t.Fatalf("approved=%+v, want status=%q reason=%q", approved[0], ApprovalStatusApproved, "ok")
	}

	pending, err := store.ListApprovalRequestsByTask(ctx, task.ID, ListApprovalRequestsOptions{Status: ApprovalStatusPending})
	if err != nil {
		t.Fatalf("ListApprovalRequestsByTask pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending len=%d, want 0", len(pending))
	}

	got, ok, err := store.GetApprovalRequest(ctx, ar.ID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
	if got.ID != ar.ID || got.TaskID != task.ID {
		t.Fatalf("got=%+v, want id=%q task_id=%q", got, ar.ID, task.ID)
	}
}
