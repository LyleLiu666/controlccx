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
	return "创建或更新任务契约（mission contract）。参数：key 或 task_id（二选一，建议 task_id）；goal（可选，创建/更新时必填）；constraints / acceptance_criteria / non_goals（可选，逗号或换行分隔）；confirm（可选，true 表示确认当前 revision，可与更新同发或单独确认）。"
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
	confirm := parseBool(call.Fields["confirm"])
	if goal == "" && !confirm {
		return nil, errors.New("goal is required")
	}

	var (
		contract  tasks.MissionContract
		err       error
		confirmed bool
	)
	if goal != "" {
		contract, err = deps.Tasks.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
			Key:                key,
			Goal:               goal,
			Constraints:        parseStringSliceCSV(call.Fields["constraints"]),
			AcceptanceCriteria: parseStringSliceCSV(call.Fields["acceptance_criteria"]),
			NonGoals:           parseStringSliceCSV(call.Fields["non_goals"]),
		})
		if err != nil {
			return nil, err
		}
	} else {
		var ok bool
		contract, ok, err = deps.Tasks.GetMissionContract(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("mission contract not found")
		}
	}

	if confirm {
		if _, err := deps.Tasks.ConfirmMissionContract(ctx, key); err != nil {
			return nil, err
		}
		confirmed = true
	}
	return map[string]any{
		"contract":  contract,
		"confirmed": confirmed,
	}, nil
}
