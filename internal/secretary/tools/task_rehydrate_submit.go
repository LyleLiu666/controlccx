package tools

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskRehydrateSubmitTool struct{}

func (taskRehydrateSubmitTool) Name() string { return "task_rehydrate_submit" }

func (taskRehydrateSubmitTool) DescriptionZH() string {
	return "按 task_id 提交 rehydrate（基于持久化上下文新建run）。参数：task_id（必填）、prompt（可选）和可选安全参数。"
}

func (taskRehydrateSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	body := runOptionsFromFields(call.Fields)
	t, err := ops.RehydrateTask(ctx, taskID, body)
	ops.AppendActionAuditLog(ctx, taskID, "task_rehydrate_submit", map[string]any{"task_id": taskID, "prompt": body.Prompt}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": t}, nil
}
