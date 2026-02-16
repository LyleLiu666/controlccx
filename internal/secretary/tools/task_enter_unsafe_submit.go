package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskEnterUnsafeSubmitTool struct{}

func (taskEnterUnsafeSubmitTool) Name() string { return "task_enter_unsafe_submit" }

func (taskEnterUnsafeSubmitTool) DescriptionZH() string {
	return "提交高风险继续（enter-unsafe）。参数：task_id（必填）、confirm（必填，必须true）、prompt（可选，默认continue）。"
}

func (taskEnterUnsafeSubmitTool) Params() []string {
	return []string{"task_id", "confirm", "prompt"}
}

func (taskEnterUnsafeSubmitTool) Required() []string { return []string{"task_id", "confirm"} }

func (taskEnterUnsafeSubmitTool) AnyOfRequired() [][]string { return nil }

func (taskEnterUnsafeSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	if !parseBool(call.Fields["confirm"]) {
		err := errors.New("confirm=true is required for enter-unsafe")
		ops.AppendActionAuditLog(ctx, taskID, "task_enter_unsafe_submit", map[string]any{"task_id": taskID}, err)
		return nil, err
	}
	prompt := strings.TrimSpace(call.Fields["prompt"])
	t, err := ops.EnterUnsafeTask(ctx, taskID, prompt)
	ops.AppendActionAuditLog(ctx, taskID, "task_enter_unsafe_submit", map[string]any{"task_id": taskID, "prompt": prompt, "confirm": true}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": t}, nil
}
