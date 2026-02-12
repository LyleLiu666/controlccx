package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
)

type projectAutonomyPolicyUpsertTool struct{}

func (projectAutonomyPolicyUpsertTool) Name() string { return "project_autonomy_policy_upsert" }

func (projectAutonomyPolicyUpsertTool) DescriptionZH() string {
	return "设置项目自治策略。参数：task_id 或 workdir（二选一）；mode（可选，graded|max，默认 graded）。"
}

func (projectAutonomyPolicyUpsertTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	workdir := strings.TrimSpace(call.Fields["workdir"])
	if workdir == "" {
		if taskID == "" {
			return nil, errors.New("task_id or workdir is required")
		}
		task, err := deps.Tasks.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		workdir = strings.TrimSpace(task.WorkDir)
	}

	policy, err := deps.Tasks.UpsertProjectAutonomyPolicy(ctx, workdir, strings.TrimSpace(call.Fields["mode"]))
	if err != nil {
		return nil, err
	}
	return map[string]any{"policy": policy}, nil
}

