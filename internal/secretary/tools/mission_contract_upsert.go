package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type missionContractUpsertTool struct{}

func (missionContractUpsertTool) Name() string { return "mission_contract_upsert" }

func (missionContractUpsertTool) DescriptionZH() string {
	return "创建或更新任务契约（mission contract）。参数：key 或 task_id（二选一，建议 task_id）；goal（必填）；constraints / acceptance_criteria / non_goals（可选，逗号或换行分隔）。"
}

func (missionContractUpsertTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	key := strings.TrimSpace(call.Fields["key"])
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if key == "" {
		if taskID == "" {
			return nil, errors.New("key or task_id is required")
		}
		task, err := deps.Tasks.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		key = tasks.SessionKeyForTask(task)
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("cannot resolve mission contract key from task")
		}
	}
	goal := strings.TrimSpace(call.Fields["goal"])
	if goal == "" {
		return nil, errors.New("goal is required")
	}

	contract, err := deps.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:                key,
		Goal:               goal,
		Constraints:        parseStringSliceCSV(call.Fields["constraints"]),
		AcceptanceCriteria: parseStringSliceCSV(call.Fields["acceptance_criteria"]),
		NonGoals:           parseStringSliceCSV(call.Fields["non_goals"]),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"contract": contract}, nil
}
