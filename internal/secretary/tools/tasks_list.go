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
	return "列出最近任务摘要。参数：limit（可选，默认50，最大200）、include_deleted（可选，1/true 表示包含已删除会话）；task_id 或 conversation_id（可选，提供后仅返回同项目范围任务，避免跨项目上下文污染）。"
}

func (tasksListTool) Params() []string {
	return []string{"limit", "include_deleted", "task_id", "conversation_id"}
}

func (tasksListTool) Required() []string { return nil }

func (tasksListTool) AnyOfRequired() [][]string { return nil }

type taskSummary struct {
	ID        string           `json:"id"`
	Status    tasks.Status     `json:"status"`
	Worker    tasks.WorkerType `json:"worker_type"`
	Prompt    string           `json:"prompt"`
	WorkDir   string           `json:"workdir"`
	UpdatedAt time.Time        `json:"updated_at"`
	CreatedAt time.Time        `json:"created_at"`
}

func taskProjectScope(t tasks.Task) string {
	if base := tasks.NormalizeProjectKey(strings.TrimSpace(t.BaseWorkDir)); base != "" {
		return base
	}
	if workdir := tasks.NormalizeProjectKey(strings.TrimSpace(t.WorkDir)); workdir != "" {
		return workdir
	}
	if cid := strings.TrimSpace(t.ConversationID); cid != "" {
		return cid
	}
	return strings.TrimSpace(t.ID)
}

func tasksListProjectScope(ctx context.Context, call agentsdk.ToolCall, store *tasks.Store) (string, bool, error) {
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID != "" {
		t, err := store.GetTask(ctx, taskID)
		if err != nil {
			return "", false, err
		}
		return taskProjectScope(t), true, nil
	}

	conversationID := strings.TrimSpace(call.Fields["conversation_id"])
	if conversationID != "" {
		scope, err := store.ProjectScopeForConversation(ctx, conversationID)
		if err != nil {
			return "", false, err
		}
		scope = tasks.NormalizeProjectKey(strings.TrimSpace(scope))
		if scope == "" {
			scope = conversationID
		}
		return scope, true, nil
	}
	return "", false, nil
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
	projectScope, scoped, err := tasksListProjectScope(ctx, call, deps.Tasks)
	if err != nil {
		return nil, err
	}

	scanLimit := limit
	if scoped && scanLimit < 500 {
		scanLimit = 500
	}
	list, err := deps.Tasks.ListTasksWithOptions(ctx, scanLimit, tasks.ListTasksOptions{IncludeDeleted: includeDeleted})
	if err != nil {
		return nil, err
	}
	out := make([]taskSummary, 0, limit)
	for _, t := range list {
		if scoped && taskProjectScope(t) != projectScope {
			continue
		}
		out = append(out, taskSummary{
			ID:        t.ID,
			Status:    t.Status,
			Worker:    t.WorkerType,
			Prompt:    truncateDisplay(strings.TrimSpace(t.Prompt), 240),
			WorkDir:   t.WorkDir,
			UpdatedAt: t.UpdatedAt,
			CreatedAt: t.CreatedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	res := map[string]any{"tasks": out}
	if scoped {
		res["project_scope"] = projectScope
	}
	return res, nil
}
