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

func TestRunExecutionPlanLoopV1_PersistsProgressAcrossIterations(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	baseTask, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-plan-loop",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-plan-loop",
	})
	if err != nil {
		t.Fatalf("create base task: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, baseTask.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  baseTask.SessionID,
		FinishedAt: baseTask.CreatedAt,
	}); err != nil {
		t.Fatalf("finish base task: %v", err)
	}

	contractKey := tasks.ConversationKey(baseTask.ConversationID)
	if _, err := svc.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  contractKey,
		Goal: "Deliver autonomous execution",
		AcceptanceCriteria: []string{
			"run required tests",
			"document key behavior changes",
		},
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := svc.Tasks.ConfirmMissionContract(ctx, contractKey); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	key := tasks.SessionKeyForTask(baseTask)
	first, err := svc.RunExecutionPlanLoopV1(ctx, key, RunExecutionPlanLoopInput{MaxIterations: 1})
	if err != nil {
		t.Fatalf("run loop first: %v", err)
	}
	if first.State.Iteration != 1 {
		t.Fatalf("first iteration=%d, want 1", first.State.Iteration)
	}
	if strings.TrimSpace(first.State.PlanJSON) == "" {
		t.Fatalf("expected non-empty plan_json")
	}
	if strings.TrimSpace(first.LastTaskID) == "" {
		t.Fatalf("expected last_task_id after first iteration")
	}

	if err := svc.Tasks.FinishTask(ctx, first.LastTaskID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "step-1 done",
		SessionID:  baseTask.SessionID,
		FinishedAt: baseTask.CreatedAt,
	}); err != nil {
		t.Fatalf("finish first loop task: %v", err)
	}

	second, err := svc.RunExecutionPlanLoopV1(ctx, key, RunExecutionPlanLoopInput{MaxIterations: 1})
	if err != nil {
		t.Fatalf("run loop second: %v", err)
	}
	if second.State.Iteration != 2 {
		t.Fatalf("second iteration=%d, want 2", second.State.Iteration)
	}
	if strings.TrimSpace(second.State.PlanJSON) != strings.TrimSpace(first.State.PlanJSON) {
		t.Fatalf("plan_json changed unexpectedly")
	}

	stored, ok, err := svc.Tasks.GetExecutionPlanState(ctx, contractKey)
	if err != nil {
		t.Fatalf("get execution plan state: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted execution plan state")
	}
	if stored.Iteration != 2 {
		t.Fatalf("stored iteration=%d, want 2", stored.Iteration)
	}
}

func TestRunExecutionPlanLoopV1_RequiresConfirmedMissionContract(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	baseTask, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-plan-loop-unconfirmed",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-plan-loop-unconfirmed",
	})
	if err != nil {
		t.Fatalf("create base task: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, baseTask.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  baseTask.SessionID,
		FinishedAt: baseTask.CreatedAt,
	}); err != nil {
		t.Fatalf("finish base task: %v", err)
	}
	if _, err := svc.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  tasks.ConversationKey(baseTask.ConversationID),
		Goal: "Deliver autonomous execution",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}

	_, err = svc.RunExecutionPlanLoopV1(ctx, tasks.SessionKeyForTask(baseTask), RunExecutionPlanLoopInput{MaxIterations: 1})
	if err == nil {
		t.Fatalf("expected unconfirmed mission contract error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "confirmed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExecutionPlanLoopV1_StopsAtConfiguredLimitAndReturnsProgress(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	baseTask, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-plan-loop-limit",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-plan-loop-limit",
	})
	if err != nil {
		t.Fatalf("create base task: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, baseTask.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  baseTask.SessionID,
		FinishedAt: baseTask.CreatedAt,
	}); err != nil {
		t.Fatalf("finish base task: %v", err)
	}

	contractKey := tasks.ConversationKey(baseTask.ConversationID)
	if _, err := svc.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  contractKey,
		Goal: "Deliver autonomous execution",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := svc.Tasks.ConfirmMissionContract(ctx, contractKey); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	key := tasks.SessionKeyForTask(baseTask)
	first, err := svc.RunExecutionPlanLoopV1(ctx, key, RunExecutionPlanLoopInput{
		MaxIterations:      1,
		MaxTotalIterations: 1,
	})
	if err != nil {
		t.Fatalf("run loop first: %v", err)
	}
	if first.IterationsExecuted != 1 {
		t.Fatalf("iterations_executed=%d, want 1", first.IterationsExecuted)
	}
	if strings.TrimSpace(first.LastTaskID) == "" {
		t.Fatalf("expected last_task_id")
	}
	if err := svc.Tasks.FinishTask(ctx, first.LastTaskID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "step-1 done",
		SessionID:  baseTask.SessionID,
		FinishedAt: baseTask.CreatedAt,
	}); err != nil {
		t.Fatalf("finish first loop task: %v", err)
	}

	second, err := svc.RunExecutionPlanLoopV1(ctx, key, RunExecutionPlanLoopInput{
		MaxIterations:      1,
		MaxTotalIterations: 1,
	})
	if err != nil {
		t.Fatalf("run loop second: %v", err)
	}
	if !second.LimitReached {
		t.Fatalf("expected limit_reached=true")
	}
	if second.Handoff == nil {
		t.Fatalf("expected handoff payload")
	}
	if second.Handoff.Action != "human_handoff" {
		t.Fatalf("handoff.action=%q, want human_handoff", second.Handoff.Action)
	}
	if strings.TrimSpace(second.Handoff.Summary) == "" {
		t.Fatalf("expected handoff summary")
	}
	if len(second.Handoff.Blockers) == 0 || len(second.Handoff.SuggestedActions) == 0 {
		t.Fatalf("expected blockers and suggested actions in handoff")
	}
	if second.IterationsExecuted != 0 {
		t.Fatalf("iterations_executed=%d, want 0", second.IterationsExecuted)
	}
	if len(second.Progress) == 0 {
		t.Fatalf("expected non-empty progress history")
	}
	if second.Progress[0].Status != "limit_reached" {
		t.Fatalf("latest progress status=%q, want limit_reached", second.Progress[0].Status)
	}
}

func TestRunExecutionPlanLoopV1_ProjectPolicyAffectsBudget(t *testing.T) {
	ctx, svc := newServiceForTest(t)
	projA := filepath.Join(t.TempDir(), "proj-a")
	baseA, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-policy-a",
		Prompt:         "seed",
		WorkDir:        projA,
		SessionID:      "sess-policy-a",
	})
	if err != nil {
		t.Fatalf("create task A: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, baseA.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  baseA.SessionID,
		FinishedAt: baseA.CreatedAt,
	}); err != nil {
		t.Fatalf("finish task A: %v", err)
	}
	if _, err := svc.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  tasks.ConversationKey(baseA.ConversationID),
		Goal: "Deliver autonomous execution",
	}); err != nil {
		t.Fatalf("upsert mission contract A: %v", err)
	}
	if _, err := svc.Tasks.ConfirmMissionContract(ctx, tasks.ConversationKey(baseA.ConversationID)); err != nil {
		t.Fatalf("confirm mission contract A: %v", err)
	}

	graded, err := svc.RunExecutionPlanLoopV1(ctx, tasks.SessionKeyForTask(baseA), RunExecutionPlanLoopInput{
		MaxIterations:      3,
		MaxTotalIterations: 20,
	})
	if err != nil {
		t.Fatalf("run loop A: %v", err)
	}
	if graded.ProjectPolicyMode != tasks.AutonomyModeGraded {
		t.Fatalf("policy mode=%q, want %q", graded.ProjectPolicyMode, tasks.AutonomyModeGraded)
	}
	if graded.IterationsRequested != 1 {
		t.Fatalf("iterations_requested=%d, want 1 in graded mode", graded.IterationsRequested)
	}
	if graded.MaxTotalIterations != 10 {
		t.Fatalf("max_total_iterations=%d, want 10 in graded mode", graded.MaxTotalIterations)
	}

	projB := filepath.Join(t.TempDir(), "proj-b")
	baseB, err := svc.Tasks.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-policy-b",
		Prompt:         "seed",
		WorkDir:        projB,
		SessionID:      "sess-policy-b",
	})
	if err != nil {
		t.Fatalf("create task B: %v", err)
	}
	if err := svc.Tasks.FinishTask(ctx, baseB.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  baseB.SessionID,
		FinishedAt: baseB.CreatedAt,
	}); err != nil {
		t.Fatalf("finish task B: %v", err)
	}
	if _, err := svc.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  tasks.ConversationKey(baseB.ConversationID),
		Goal: "Deliver autonomous execution",
	}); err != nil {
		t.Fatalf("upsert mission contract B: %v", err)
	}
	if _, err := svc.Tasks.ConfirmMissionContract(ctx, tasks.ConversationKey(baseB.ConversationID)); err != nil {
		t.Fatalf("confirm mission contract B: %v", err)
	}
	if _, err := svc.Tasks.UpsertProjectAutonomyPolicy(ctx, projB, tasks.AutonomyModeMax); err != nil {
		t.Fatalf("upsert project policy B: %v", err)
	}

	maxMode, err := svc.RunExecutionPlanLoopV1(ctx, tasks.SessionKeyForTask(baseB), RunExecutionPlanLoopInput{
		MaxIterations:      3,
		MaxTotalIterations: 20,
	})
	if err != nil {
		t.Fatalf("run loop B: %v", err)
	}
	if maxMode.ProjectPolicyMode != tasks.AutonomyModeMax {
		t.Fatalf("policy mode=%q, want %q", maxMode.ProjectPolicyMode, tasks.AutonomyModeMax)
	}
	if maxMode.IterationsRequested != 3 {
		t.Fatalf("iterations_requested=%d, want 3 in max mode", maxMode.IterationsRequested)
	}
	if maxMode.MaxTotalIterations != 20 {
		t.Fatalf("max_total_iterations=%d, want 20 in max mode", maxMode.MaxTotalIterations)
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

func TestParseMutationError_TasksInvalidPrefixIsInvalidArgument(t *testing.T) {
	problem := ParseMutationError(errors.New("tasks: invalid network_tier \"bad\""))
	if problem.Error != MutationErrorInvalidArgument {
		t.Fatalf("error code=%q want %q", problem.Error, MutationErrorInvalidArgument)
	}
	if problem.Status != 400 {
		t.Fatalf("status=%d want %d", problem.Status, 400)
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
