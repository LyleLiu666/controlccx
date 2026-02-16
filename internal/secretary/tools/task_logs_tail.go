package tools

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type taskLogsTailTool struct{}

func (taskLogsTailTool) Name() string { return "task_logs_tail" }

func (taskLogsTailTool) DescriptionZH() string {
	return "查看任务最近日志（受限）。参数：task_id（必填）、count（可选，默认5，最大20）。每条最多800字（头200+尾600）。"
}

func (taskLogsTailTool) ReturnsZH() string {
	return "task_id、count、logs[]（id/time/stream/message/truncated/original_chars），以及截断与限制信息"
}

func (taskLogsTailTool) Params() []string {
	return []string{"task_id", "count"}
}

func (taskLogsTailTool) Required() []string { return []string{"task_id"} }

func (taskLogsTailTool) AnyOfRequired() [][]string { return nil }

func (taskLogsTailTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	count := parseInt(call.Fields["count"], 5)
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}

	all, err := deps.Tasks.ListLogsTail(ctx, taskID, count)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return map[string]any{"task_id": taskID, "logs": []any{}, "count": 0}, nil
	}
	list := all
	type item struct {
		ID            int64           `json:"id"`
		Time          string          `json:"time"`
		Stream        tasks.LogStream `json:"stream"`
		Message       string          `json:"message"`
		Truncated     bool            `json:"truncated"`
		OriginalChars int             `json:"original_chars"`
	}
	out := make([]item, 0, len(list))
	for _, l := range list {
		original := strings.TrimSpace(l.Message)
		msg, trunc := truncateUTF8SafeHeadTail(l.Message, 200, 600)
		out = append(out, item{
			ID:            l.ID,
			Time:          l.Time.Format(timeRFC3339),
			Stream:        l.Stream,
			Message:       msg,
			Truncated:     trunc,
			OriginalChars: utf8.RuneCountInString(original),
		})
	}
	return map[string]any{
		"task_id":         taskID,
		"count":           len(out),
		"requested":       count,
		"max_count":       20,
		"line_max_chars":  800,
		"line_head_chars": 200,
		"line_tail_chars": 600,
		"logs":            out,
	}, nil
}
