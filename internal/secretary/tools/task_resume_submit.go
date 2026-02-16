package tools

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskResumeSubmitTool struct{}

func (taskResumeSubmitTool) Name() string { return "task_resume_submit" }

func (taskResumeSubmitTool) DescriptionZH() string {
	return "按 task_id 提交 resume run。参数：task_id（必填）、prompt（可选，默认沿用continue语义）以及可选安全参数。"
}

func (taskResumeSubmitTool) Params() []string {
	return append([]string{"task_id", "prompt"}, RunOptsParams...)
}

func (taskResumeSubmitTool) Required() []string { return []string{"task_id"} }

func (taskResumeSubmitTool) AnyOfRequired() [][]string { return nil }

func (taskResumeSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	body := runOptionsFromFields(call.Fields)
	t, err := ops.ResumeTask(ctx, taskID, body)
	ops.AppendActionAuditLog(ctx, taskID, "task_resume_submit", map[string]any{"task_id": taskID, "prompt": body.Prompt}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": t}, nil
}
