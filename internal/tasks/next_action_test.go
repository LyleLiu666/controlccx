package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_ComputeNextAction_DeterministicPriority(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "conv-priority"

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerClaudeCode,
		Mode:           ModeNew,
		ConversationID: conversationID,
		Prompt:         "seed",
		WorkDir:        filepath.Join(t.TempDir(), "proj-priority"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.SetAwaitingApproval(ctx, task.ID); err != nil {
		t.Fatalf("set awaiting approval: %v", err)
	}
	ar, err := store.CreateApprovalRequest(ctx, CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  RiskHigh,
		Summary:    "needs approval",
		Raw:        []byte(`{"cmd":"rm -rf"}`),
	})
	if err != nil {
		t.Fatalf("create approval request: %v", err)
	}

	// Also provision an active workspace; approval should still win by priority.
	if _, err := store.UpsertSessionWorkspace(ctx, UpsertSessionWorkspaceInput{
		Key:         SessionKeyForTask(task),
		Kind:        "copy",
		BaseWorkDir: task.WorkDir,
		RunRoot:     filepath.Join(task.WorkDir, ".ccx", "workspaces", "conv-priority"),
		RunWorkDir:  filepath.Join(task.WorkDir, ".ccx", "workspaces", "conv-priority"),
		Status:      "active",
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}

	next, err := store.ComputeNextAction(ctx, conversationID)
	if err != nil {
		t.Fatalf("compute next action: %v", err)
	}
	if next.Action != NextActionResolveApproval {
		t.Fatalf("action=%q, want %q", next.Action, NextActionResolveApproval)
	}
	if next.Reason != "pending_approval" {
		t.Fatalf("reason=%q, want %q", next.Reason, "pending_approval")
	}
	if next.TaskID != task.ID {
		t.Fatalf("task_id=%q, want %q", next.TaskID, task.ID)
	}
	if next.ApprovalID != ar.ID {
		t.Fatalf("approval_id=%q, want %q", next.ApprovalID, ar.ID)
	}
}

func TestStore_ComputeNextAction_ResumeAfterFailure(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "conv-failed"

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerCodex,
		Mode:           ModeNew,
		ConversationID: conversationID,
		Prompt:         "seed",
		WorkDir:        filepath.Join(t.TempDir(), "proj-failed"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusFailed,
		Error:      "boom",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task failed: %v", err)
	}

	next, err := store.ComputeNextAction(ctx, conversationID)
	if err != nil {
		t.Fatalf("compute next action: %v", err)
	}
	if next.Action != NextActionResumeRun {
		t.Fatalf("action=%q, want %q", next.Action, NextActionResumeRun)
	}
	if next.Reason != "latest_failed" {
		t.Fatalf("reason=%q, want %q", next.Reason, "latest_failed")
	}
}

func TestStore_ComputeNextAction_WorkspaceMergeWhenNoInFlight(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "conv-workspace"

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerClaudeCode,
		Mode:           ModeNew,
		ConversationID: conversationID,
		Prompt:         "seed",
		WorkDir:        filepath.Join(t.TempDir(), "proj-workspace"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	if _, err := store.UpsertSessionWorkspace(ctx, UpsertSessionWorkspaceInput{
		Key:         SessionKeyForTask(task),
		Kind:        "copy",
		BaseWorkDir: task.WorkDir,
		RunRoot:     filepath.Join(task.WorkDir, ".ccx", "workspaces", "conv-workspace"),
		RunWorkDir:  filepath.Join(task.WorkDir, ".ccx", "workspaces", "conv-workspace"),
		Status:      "active",
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}

	next, err := store.ComputeNextAction(ctx, conversationID)
	if err != nil {
		t.Fatalf("compute next action: %v", err)
	}
	if next.Action != NextActionMergeWorkspace {
		t.Fatalf("action=%q, want %q", next.Action, NextActionMergeWorkspace)
	}
	if next.Reason != "workspace_active" {
		t.Fatalf("reason=%q, want %q", next.Reason, "workspace_active")
	}
}

func TestStore_ComputeNextAction_ContractConfirmationGate(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "conv-contract-gate"

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:     WorkerClaudeCode,
		Mode:           ModeNew,
		ConversationID: conversationID,
		Prompt:         "seed",
		WorkDir:        filepath.Join(t.TempDir(), "proj-contract-gate"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusFailed,
		Error:      "boom",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	if _, err := store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  ConversationKey(conversationID),
		Goal: "Deliver autonomous loop",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}

	next, err := store.ComputeNextAction(ctx, conversationID)
	if err != nil {
		t.Fatalf("compute next action: %v", err)
	}
	if next.Action != NextActionConfirmContract {
		t.Fatalf("action=%q, want %q", next.Action, NextActionConfirmContract)
	}
	if next.Reason != "contract_unconfirmed" {
		t.Fatalf("reason=%q, want %q", next.Reason, "contract_unconfirmed")
	}

	if _, err := store.ConfirmMissionContract(ctx, ConversationKey(conversationID)); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	nextAfterConfirm, err := store.ComputeNextAction(ctx, conversationID)
	if err != nil {
		t.Fatalf("compute next action after confirm: %v", err)
	}
	if nextAfterConfirm.Action != NextActionResumeRun {
		t.Fatalf("action=%q, want %q", nextAfterConfirm.Action, NextActionResumeRun)
	}
	if nextAfterConfirm.Reason != "latest_failed" {
		t.Fatalf("reason=%q, want %q", nextAfterConfirm.Reason, "latest_failed")
	}
}
