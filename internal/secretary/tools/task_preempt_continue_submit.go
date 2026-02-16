package tools

import (
	"context"

	"controlccx/internal/agentsdk"
)

type taskPreemptContinueSubmitTool struct{}

func (taskPreemptContinueSubmitTool) Name() string { return "task_preempt_continue_submit" }

func (taskPreemptContinueSubmitTool) DescriptionZH() string {
	return "抢占继续：尝试取消当前会话 in-flight run，并把新 continue 以更高优先级入队。参数：task_id（必填）、prompt（可选）和可选安全参数。"
}

func (taskPreemptContinueSubmitTool) Params() []string {
	return append([]string{"task_id", "prompt"}, RunOptsParams...)
}

func (taskPreemptContinueSubmitTool) Required() []string { return []string{"task_id"} }

func (taskPreemptContinueSubmitTool) AnyOfRequired() [][]string { return nil }

func (taskPreemptContinueSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID, key, _, err := resolveSessionByTaskID(ctx, ops, call.Fields)
	if err != nil {
		return nil, err
	}
	body := runOptionsFromFields(call.Fields)
	ack, err := ops.PreemptContinueSession(ctx, key, body)
	ops.AppendActionAuditLog(ctx, taskID, "task_preempt_continue_submit", map[string]any{"task_id": taskID, "session_key": key, "prompt": body.Prompt}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"queued":            ack.Queued,
		"queue_id":          ack.QueueID,
		"position":          ack.Position,
		"preempted_task_id": ack.PreemptedTaskID,
	}, nil
}
