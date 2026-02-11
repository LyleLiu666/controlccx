package tools

import (
	"context"
	"errors"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type tasksListTool struct{}

func (tasksListTool) Name() string { return "tasks_list" }

func (tasksListTool) DescriptionZH() string {
	return "列出最近任务摘要。参数：limit（可选，默认50，最大200）、include_deleted（可选，1/true 表示包含已删除会话）。"
}

type taskSummary struct {
	ID        string           `json:"id"`
	Status    tasks.Status     `json:"status"`
	Worker    tasks.WorkerType `json:"worker_type"`
	Prompt    string           `json:"prompt"`
	WorkDir   string           `json:"workdir"`
	UpdatedAt time.Time        `json:"updated_at"`
	CreatedAt time.Time        `json:"created_at"`
}

func (tasksListTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	limit := parseInt(call.Fields["limit"], 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	includeDeleted := parseBool(call.Fields["include_deleted"])

	list, err := deps.Tasks.ListTasksWithOptions(ctx, limit, tasks.ListTasksOptions{IncludeDeleted: includeDeleted})
	if err != nil {
		return nil, err
	}
	out := make([]taskSummary, 0, len(list))
	for _, t := range list {
		out = append(out, taskSummary{
			ID:        t.ID,
			Status:    t.Status,
			Worker:    t.WorkerType,
			Prompt:    truncateDisplay(strings.TrimSpace(t.Prompt), 240),
			WorkDir:   t.WorkDir,
			UpdatedAt: t.UpdatedAt,
			CreatedAt: t.CreatedAt,
		})
	}
	return map[string]any{"tasks": out}, nil
}
