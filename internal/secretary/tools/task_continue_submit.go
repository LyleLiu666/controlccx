package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskContinueSubmitTool struct{}

func (taskContinueSubmitTool) Name() string { return "task_continue_submit" }

func (taskContinueSubmitTool) DescriptionZH() string {
	return "提交会话继续请求。参数：task_id（必填，定位会话）、prompt（可选，默认continue）以及可选安全参数。"
}

func (taskContinueSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID, key, _, err := resolveSessionByTaskID(ctx, ops, call.Fields)
	if err != nil {
		return nil, err
	}
	body := runOptionsFromFields(call.Fields)
	{
		p := strings.TrimSpace(body.Prompt)
		lp := strings.ToLower(p)
		// Do not overload "continue" with "cancel" semantics: this created a severe UX bug.
		if p == "/cancel" || lp == "cancel" || p == "取消" {
			err := errors.New("cancel prompt is not supported; use task_cancel_submit instead")
			ops.AppendActionAuditLog(ctx, taskID, "task_continue_submit", map[string]any{"task_id": taskID, "session_key": key, "prompt": body.Prompt}, err)
			return nil, err
		}
	}
	res, err := ops.ContinueSession(ctx, key, body)
	ops.AppendActionAuditLog(ctx, taskID, "task_continue_submit", map[string]any{"task_id": taskID, "session_key": key, "prompt": body.Prompt}, err)
	if err != nil {
		return nil, err
	}
	if res.Queue != nil {
		return map[string]any{
			"queued":           res.Queue.Queued,
			"queue_id":         res.Queue.QueueID,
			"position":         res.Queue.Position,
			"existing_task_id": res.Queue.ExistingTaskID,
			"existing_status":  res.Queue.ExistingStatus,
		}, nil
	}
	if res.Task != nil {
		return map[string]any{"task": *res.Task}, nil
	}
	return map[string]any{"ok": true}, nil
}
