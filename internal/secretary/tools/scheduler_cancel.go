package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
)

type schedulerCancelTool struct{}

func (schedulerCancelTool) Name() string { return "scheduler_cancel" }

func (schedulerCancelTool) DescriptionZH() string {
	return "取消一个调度任务。参数：schedule_id（必填）。"
}

func (schedulerCancelTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	scheduler, err := requireScheduler(deps)
	if err != nil {
		return nil, err
	}
	scheduleID := strings.TrimSpace(call.Fields["schedule_id"])
	if scheduleID == "" {
		return nil, errors.New("schedule_id is required")
	}
	info, err := scheduler.CancelSchedule(withBackgroundContext(ctx), scheduleID)
	if err != nil {
		return nil, err
	}
	return scheduleInfoToResult(info), nil
}
