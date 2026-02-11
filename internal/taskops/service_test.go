package taskops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

type approvalSyncRunner struct {
	store *tasks.Store
}

func (approvalSyncRunner) Start(ctx context.Context, taskID string) error { return nil }

func (approvalSyncRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	return false, nil
}

func (r approvalSyncRunner) SubmitApprovalDecision(ctx context.Context, taskID string, approvalID string, decision string, reason string) error {
	var status tasks.ApprovalStatus
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve":
		status = tasks.ApprovalStatusApproved
	case "deny":
		status = tasks.ApprovalStatusDenied
	default:
		return errors.New("invalid decision")
	}
	return r.store.UpdateApprovalRequestDecision(ctx, approvalID, tasks.UpdateApprovalRequestDecisionInput{
		Status: status,
		Reason: strings.TrimSpace(reason),
	})
}

func newServiceForTest(t *testing.T) (context.Context, *Service) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	store := tasks.NewStore(conn)
	return ctx, &Service{Tasks: store}
}

func TestContinueSession_QueuesWhenInFlight(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	first, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := svc.Tasks.SetRunning(ctx, first.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	out, err := svc.ContinueSession(ctx, tasks.SessionKeyForTask(first), RunOptions{Prompt: "continue"})
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	if out.Queue == nil || !out.Queue.Queued {
		t.Fatalf("expected queued ack, got: %#v", out)
	}
	if strings.TrimSpace(out.Queue.ExistingTaskID) != first.ID {
		t.Fatalf("existing_task_id=%q want %q", out.Queue.ExistingTaskID, first.ID)
	}
}

func TestDecideApproval_AutoResolvePending(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	task, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ar, err := svc.Tasks.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "s",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	out, err := svc.DecideApproval(ctx, task.ID, "", "approve", "ok")
	if err != nil {
		t.Fatalf("decide approval: %v", err)
	}
	if out.ID != ar.ID {
		t.Fatalf("approval id=%q want %q", out.ID, ar.ID)
	}
	if out.Status != tasks.ApprovalStatusApproved {
		t.Fatalf("status=%q want approved", out.Status)
	}
}

func TestDecideApproval_WithForwarderAlreadyUpdated_RemainsSuccessful(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	task, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ar, err := svc.Tasks.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "s",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}

	svc.Workers = approvalSyncRunner{store: svc.Tasks}

	out, err := svc.DecideApproval(ctx, task.ID, ar.ID, "approve", "ok")
	if err != nil {
		t.Fatalf("decide approval: %v", err)
	}
	if out.Status != tasks.ApprovalStatusApproved {
		t.Fatalf("status=%q want approved", out.Status)
	}
	if out.Reason != "ok" {
		t.Fatalf("reason=%q want %q", out.Reason, "ok")
	}
}
