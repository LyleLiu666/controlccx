package tools

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskCancelSubmitTool struct{}

func (taskCancelSubmitTool) Name() string { return "task_cancel_submit" }

func (taskCancelSubmitTool) DescriptionZH() string {
	return "取消一个任务（当前 run）。参数：task_id（必填）。"
}

func (taskCancelSubmitTool) Params() []string {
	return []string{"task_id"}
}

func (taskCancelSubmitTool) Required() []string { return []string{"task_id"} }

func (taskCancelSubmitTool) AnyOfRequired() [][]string { return nil }

func (taskCancelSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}

	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}

	res, err := ops.CancelTask(ctx, taskID)
	ops.AppendActionAuditLog(ctx, taskID, "task_cancel_submit", map[string]any{"task_id": taskID}, err)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"task_id":                 strings.TrimSpace(res.TaskID),
		"requested":               res.Requested,
		"status_before":           string(res.StatusBefore),
		"status_after":            string(res.StatusAfter),
		"runner_cancel_attempted": res.RunnerCancelAttempted,
		"runner_cancel_ok":        res.RunnerCancelOK,
		"promoted_task_id":        strings.TrimSpace(res.PromotedTaskID),
		"started_task_id":         strings.TrimSpace(res.StartedTaskID),
		"next_start_error":        strings.TrimSpace(res.NextStartError),
	}, nil
}
