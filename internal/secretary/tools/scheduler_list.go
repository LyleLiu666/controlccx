package tools

import (
	"context"

	"controlccx/internal/agentsdk"
)

type schedulerListTool struct{}

func (schedulerListTool) Name() string { return "scheduler_list" }

func (schedulerListTool) DescriptionZH() string {
	return "列出当前内存中的活跃调度任务。"
}

func (schedulerListTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = call
	scheduler, err := requireScheduler(deps)
	if err != nil {
		return nil, err
	}
	list, err := scheduler.ListSchedules(withBackgroundContext(ctx))
	if err != nil {
		return nil, err
	}
	return map[string]any{"schedules": list}, nil
}
