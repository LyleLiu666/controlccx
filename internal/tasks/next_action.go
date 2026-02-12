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
		return out, nil
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
			return out, nil
		}
		out.Action = NextActionWaitInFlight
		out.Reason = "in_flight_" + strings.TrimSpace(string(task.Status))
		out.TaskID = task.ID
		return out, nil
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
			return out, nil
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
	return out, nil
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
