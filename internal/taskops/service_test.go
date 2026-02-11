package taskops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
)

type approvalSyncRunner struct {
	store *tasks.Store
}

type startFailRunner struct{}

func (startFailRunner) Start(ctx context.Context, taskID string) error {
	_ = ctx
	_ = taskID
	return errors.New("runner down")
}

func (startFailRunner) Cancel(ctx context.Context, taskID string) (bool, error) {
	_ = ctx
	_ = taskID
	return false, nil
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

func TestCreateTask_Success_NewMode(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	svc.Hub = events.NewHub()
	task, err := svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if strings.TrimSpace(task.ID) == "" {
		t.Fatalf("expected task id")
	}
	if task.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q want %q", task.Mode, tasks.ModeNew)
	}
}

func TestCreateTask_UnknownTool_ReturnsInvalidArgument(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	toolsSvc, err := tooling.NewService(tooling.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("new tools service: %v", err)
	}
	svc.Tools = toolsSvc

	_, err = svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerType("unknown-tool"),
		Mode:       tasks.ModeNew,
		Prompt:     "p",
		WorkDir:    ".",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	problem := ParseMutationError(err)
	if problem.Error != MutationErrorInvalidArgument {
		t.Fatalf("error code=%q want %q", problem.Error, MutationErrorInvalidArgument)
	}
}

func TestCreateTask_RunnerUnavailable_ReturnsTypedError(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	svc.Workers = startFailRunner{}

	_, err := svc.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	})
	if err == nil {
		t.Fatalf("expected runner unavailable error")
	}
	problem := ParseMutationError(err)
	if problem.Error != MutationErrorRunnerUnavailable {
		t.Fatalf("error code=%q want %q", problem.Error, MutationErrorRunnerUnavailable)
	}
	if problem.Status != 503 {
		t.Fatalf("status=%d want %d", problem.Status, 503)
	}
}

func TestMutationErrors_AreStructuredAndCodeStable(t *testing.T) {
	problem := ParseMutationError(errors.New("session_task_in_flight: existing_task_id=task-123 existing_status=running"))
	if problem.Error != MutationErrorSessionTaskInFlight {
		t.Fatalf("error code=%q want %q", problem.Error, MutationErrorSessionTaskInFlight)
	}
	if problem.Status != 409 {
		t.Fatalf("status=%d want %d", problem.Status, 409)
	}
	if got := strings.TrimSpace(fmt.Sprint(problem.Details["existing_task_id"])); got != "task-123" {
		t.Fatalf("existing_task_id=%q want %q", got, "task-123")
	}
	if got := strings.TrimSpace(fmt.Sprint(problem.Details["existing_status"])); got != "running" {
		t.Fatalf("existing_status=%q want %q", got, "running")
	}
}

func TestParseMutationError_DoesNotUseBroadStringHeuristics(t *testing.T) {
	notFoundLike := ParseMutationError(errors.New("upstream responded with unexpected field: not found in payload"))
	if notFoundLike.Error != MutationErrorInternal {
		t.Fatalf("error code=%q want %q", notFoundLike.Error, MutationErrorInternal)
	}

	sessionLike := ParseMutationError(errors.New("session worker crashed unexpectedly"))
	if sessionLike.Error != MutationErrorInternal {
		t.Fatalf("error code=%q want %q", sessionLike.Error, MutationErrorInternal)
	}
}

func TestCreateContinueTaskForConversation_UsesUnifiedSemantics(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	first, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, first.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  first.SessionID,
		FinishedAt: first.CreatedAt,
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}

	next, err := svc.CreateContinueTaskForConversation(ctx, first.ConversationID, RunOptions{Prompt: "continue"})
	if err != nil {
		t.Fatalf("create continue: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q want %q", next.Mode, tasks.ModeResume)
	}
}

func TestCreateContinueTaskForConversation_ReturnsSessionTaskInFlight(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	first, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := svc.Tasks.SetRunning(ctx, first.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	_, err = svc.CreateContinueTaskForConversation(ctx, first.ConversationID, RunOptions{Prompt: "continue"})
	if err == nil {
		t.Fatalf("expected session_task_in_flight error")
	}
	problem := ParseMutationError(err)
	if problem.Error != MutationErrorSessionTaskInFlight {
		t.Fatalf("error code=%q want %q", problem.Error, MutationErrorSessionTaskInFlight)
	}
}
