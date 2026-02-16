package tools

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/taskops"
)

type executionPlanLoopSubmitTool struct{}

func (executionPlanLoopSubmitTool) Name() string { return "execution_plan_loop_submit" }

func (executionPlanLoopSubmitTool) DescriptionZH() string {
	return "按已确认的任务契约运行执行计划循环（v1）。参数：key 或 task_id（二选一，建议 task_id）；max_iterations（可选，单次最多执行几步，默认1，最大10）；max_total_iterations（可选，总上限，默认10）；iteration_prompt（可选，单轮执行提示）。"
}

func (executionPlanLoopSubmitTool) Params() []string {
	return []string{"key", "task_id", "max_iterations", "max_total_iterations", "iteration_prompt"}
}

func (executionPlanLoopSubmitTool) Required() []string { return nil }

func (executionPlanLoopSubmitTool) AnyOfRequired() [][]string {
	return [][]string{{"key", "task_id"}}
}

func (executionPlanLoopSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}

	taskID := strings.TrimSpace(call.Fields["task_id"])
	key := strings.TrimSpace(call.Fields["key"])
	if key == "" {
		var resolvedTaskID string
		resolvedTaskID, key, _, err = resolveSessionByTaskID(ctx, ops, call.Fields)
		if err != nil {
			return nil, err
		}
		taskID = strings.TrimSpace(resolvedTaskID)
	}

	maxIterations := parseInt(call.Fields["max_iterations"], 1)
	if maxIterations <= 0 {
		maxIterations = 1
	}
	if maxIterations > 10 {
		maxIterations = 10
	}
	maxTotalIterations := parseInt(call.Fields["max_total_iterations"], 10)
	if maxTotalIterations <= 0 {
		maxTotalIterations = 10
	}
	if maxTotalIterations > 200 {
		maxTotalIterations = 200
	}

	out, runErr := ops.RunExecutionPlanLoopV1(ctx, key, taskops.RunExecutionPlanLoopInput{
		MaxIterations:      maxIterations,
		MaxTotalIterations: maxTotalIterations,
		IterationPrompt:    strings.TrimSpace(call.Fields["iteration_prompt"]),
	})
	auditTaskID := strings.TrimSpace(taskID)
	if auditTaskID == "" {
		auditTaskID = strings.TrimSpace(out.LastTaskID)
	}
	ops.AppendActionAuditLog(ctx, auditTaskID, "execution_plan_loop_submit", map[string]any{
		"key":                  key,
		"max_iterations":       maxIterations,
		"max_total_iterations": maxTotalIterations,
		"iteration_prompt":     strings.TrimSpace(call.Fields["iteration_prompt"]),
	}, runErr)
	if runErr != nil {
		return nil, runErr
	}
	return out, nil
}
