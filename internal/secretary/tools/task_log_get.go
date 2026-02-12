package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
)

type taskLogGetTool struct{}

func (taskLogGetTool) Name() string { return "task_log_get" }

func (taskLogGetTool) DescriptionZH() string {
	return "按日志ID查看单条完整日志。参数：task_id（必填）、log_id（必填）。返回最多12000字（头2000+尾10000），超出会标记truncated=true。"
}

func (taskLogGetTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	logID := int64(parseInt(call.Fields["log_id"], 0))
	if logID <= 0 {
		return nil, errors.New("log_id is required")
	}
	list, err := deps.Tasks.ListLogs(ctx, taskID, logID-1, 5)
	if err != nil {
		return nil, err
	}
	for _, l := range list {
		if l.ID != logID {
			continue
		}
		original := strings.TrimSpace(l.Message)
		msg, trunc := truncateUTF8SafeHeadTail(l.Message, 2000, 10000)
		return map[string]any{
			"task_id":        taskID,
			"log_id":         l.ID,
			"time":           l.Time.Format(timeRFC3339),
			"stream":         l.Stream,
			"message":        msg,
			"truncated":      trunc,
			"original_chars": utf8.RuneCountInString(original),
			"max_chars":      12000,
		}, nil
	}
	return nil, fmt.Errorf("log %d not found for task %s", logID, taskID)
}
