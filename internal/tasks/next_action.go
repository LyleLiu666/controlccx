package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type NextActionType string

const (
	NextActionResolveApproval NextActionType = "resolve_approval"
	NextActionWaitInFlight    NextActionType = "wait_in_flight"
	NextActionMergeWorkspace  NextActionType = "merge_workspace"
	NextActionResumeRun       NextActionType = "resume_run"
	NextActionStartRun        NextActionType = "start_run"
	NextActionConfirmContract NextActionType = "confirm_contract"
)

type NextAction struct {
	ConversationID string         `json:"conversation_id"`
	Action         NextActionType `json:"action"`
	Reason         string         `json:"reason"`
	TaskID         string         `json:"task_id,omitempty"`
	ApprovalID     string         `json:"approval_id,omitempty"`
}

func (s *Store) ComputeNextAction(ctx context.Context, conversationID string) (NextAction, error) {
	if s == nil || s.db == nil {
		return NextAction{}, errors.New("tasks: store not initialized")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return NextAction{}, errors.New("tasks: conversation_id is required")
	}
	out := NextAction{ConversationID: conversationID}

	runs, err := s.ListTasksByConversationID(ctx, conversationID, 500, ListTasksOptions{})
	if err != nil {
		return NextAction{}, err
	}
	if len(runs) == 0 {
		out.Action = NextActionStartRun
		out.Reason = "no_runs"
		return s.applyContractConfirmationGate(ctx, out)
	}

	if task, ok := pickInFlightTask(runs); ok {
		approvals, err := s.ListApprovalRequestsByTask(ctx, task.ID, ListApprovalRequestsOptions{
			Status: ApprovalStatusPending,
			Limit:  1,
		})
		if err != nil {
			return NextAction{}, fmt.Errorf("tasks: compute next action approvals: %w", err)
		}
		if len(approvals) > 0 {
			out.Action = NextActionResolveApproval
			out.Reason = "pending_approval"
			out.TaskID = task.ID
			out.ApprovalID = approvals[0].ID
			return s.applyContractConfirmationGate(ctx, out)
		}
		out.Action = NextActionWaitInFlight
		out.Reason = "in_flight_" + strings.TrimSpace(string(task.Status))
		out.TaskID = task.ID
		return s.applyContractConfirmationGate(ctx, out)
	}

	latest := runs[0]
	if key := strings.TrimSpace(SessionKeyForTask(latest)); key != "" {
		ws, ok, err := s.GetSessionWorkspace(ctx, key)
		if err != nil {
			return NextAction{}, fmt.Errorf("tasks: compute next action workspace: %w", err)
		}
		if ok && strings.EqualFold(strings.TrimSpace(ws.Status), "active") {
			out.Action = NextActionMergeWorkspace
			out.Reason = "workspace_active"
			out.TaskID = latest.ID
			return s.applyContractConfirmationGate(ctx, out)
		}
	}

	switch latest.Status {
	case StatusFailed, StatusInterrupted, StatusBlocked, StatusCanceled:
		out.Action = NextActionResumeRun
		out.Reason = "latest_" + strings.TrimSpace(string(latest.Status))
		out.TaskID = latest.ID
	default:
		out.Action = NextActionStartRun
		out.Reason = "no_blockers"
		out.TaskID = latest.ID
	}
	return s.applyContractConfirmationGate(ctx, out)
}

func pickInFlightTask(runs []Task) (Task, bool) {
	priority := []Status{
		StatusAwaitingApproval,
		StatusRunning,
		StatusQueued,
		StatusWaiting,
	}
	for _, status := range priority {
		for _, t := range runs {
			if t.Status == status {
				return t, true
			}
		}
	}
	return Task{}, false
}

func (s *Store) applyContractConfirmationGate(ctx context.Context, next NextAction) (NextAction, error) {
	if next.Action != NextActionStartRun && next.Action != NextActionResumeRun {
		return next, nil
	}
	key := ConversationKey(next.ConversationID)
	if strings.TrimSpace(key) == "" {
		return next, nil
	}
	contract, ok, err := s.GetMissionContract(ctx, key)
	if err != nil {
		return NextAction{}, fmt.Errorf("tasks: compute next action mission contract: %w", err)
	}
	if !ok {
		return next, nil
	}
	confirmed, err := s.IsMissionContractRevisionConfirmed(ctx, key, contract.Revision)
	if err != nil {
		return NextAction{}, fmt.Errorf("tasks: compute next action mission contract confirmation: %w", err)
	}
	if confirmed {
		return next, nil
	}
	next.Action = NextActionConfirmContract
	next.Reason = "contract_unconfirmed"
	next.ApprovalID = ""
	return next, nil
}
