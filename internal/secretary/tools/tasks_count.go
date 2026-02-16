package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type tasksCountTool struct{}

func (tasksCountTool) Name() string { return "tasks_count" }

func (tasksCountTool) DescriptionZH() string {
	return "统计任务数量。参数：status（可选，queued/waiting/running/succeeded/failed/canceled/interrupted/blocked/awaiting_approval）、include_deleted（可选，1/true 表示包含已删除会话）。"
}

func (tasksCountTool) Params() []string {
	return []string{"status", "include_deleted"}
}

func (tasksCountTool) Required() []string { return nil }

func (tasksCountTool) AnyOfRequired() [][]string { return nil }

var knownTaskStatusesList = []string{
	string(tasks.StatusQueued),
	string(tasks.StatusWaiting),
	string(tasks.StatusRunning),
	string(tasks.StatusAwaitingApproval),
	string(tasks.StatusSucceeded),
	string(tasks.StatusFailed),
	string(tasks.StatusCanceled),
	string(tasks.StatusInterrupted),
	string(tasks.StatusBlocked),
}

var knownTaskStatuses = func() map[string]struct{} {
	out := make(map[string]struct{}, len(knownTaskStatusesList))
	for _, st := range knownTaskStatusesList {
		s := strings.ToLower(strings.TrimSpace(st))
		if s == "" {
			continue
		}
		out[s] = struct{}{}
	}
	return out
}()

func isKnownTaskStatus(s string) bool {
	_, ok := knownTaskStatuses[strings.ToLower(strings.TrimSpace(s))]
	return ok
}

func (tasksCountTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	statusFilter := strings.ToLower(strings.TrimSpace(call.Fields["status"]))
	if statusFilter != "" && !isKnownTaskStatus(statusFilter) {
		return nil, fmt.Errorf("unknown status %q (allowed: %s)", statusFilter, strings.Join(knownTaskStatusesList, ", "))
	}
	includeDeleted := parseBool(call.Fields["include_deleted"])

	counts, total, err := deps.Tasks.CountByStatus(ctx, tasks.ListTasksOptions{IncludeDeleted: includeDeleted})
	if err != nil {
		return nil, err
	}

	by := map[string]int{}
	for st, n := range counts {
		by[string(st)] = n
	}
	if statusFilter != "" {
		filtered := by[statusFilter]
		return map[string]any{
			"total":     filtered,
			"by_status": map[string]int{statusFilter: filtered},
		}, nil
	}
	return map[string]any{
		"total":     total,
		"by_status": by,
	}, nil
}
