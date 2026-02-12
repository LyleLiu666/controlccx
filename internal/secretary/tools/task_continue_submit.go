package tools

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
)

func looksLikeCancelPrompt(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}

	lower := strings.ToLower(prompt)
	// Slash-prefixed commands are unambiguous; treat any "/cancel..." variant as a cancel attempt.
	if strings.HasPrefix(lower, "/cancel") {
		return true
	}

	stripLower := strings.TrimSpace(strings.TrimRightFunc(lower, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}))
	if stripLower == "cancel" {
		return true
	}

	stripZH := strings.TrimSpace(strings.TrimRightFunc(prompt, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}))
	if stripZH == "取消" {
		return true
	}
	// Short Chinese variants like "取消一下" are also likely cancel attempts.
	if strings.HasPrefix(stripZH, "取消") {
		n := utf8.RuneCountInString(stripZH)
		if n > 0 && n <= 6 {
			return true
		}
	}

	return false
}

type taskContinueSubmitTool struct{}

func (taskContinueSubmitTool) Name() string { return "task_continue_submit" }

func (taskContinueSubmitTool) DescriptionZH() string {
	return "提交会话继续请求。参数：task_id（必填，定位会话）、prompt（可选，默认continue）以及可选安全参数。"
}

func (taskContinueSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID, key, _, err := resolveSessionByTaskID(ctx, ops, call.Fields)
	if err != nil {
		return nil, err
	}
	body := runOptionsFromFields(call.Fields)
	{
		// Do not overload "continue" with "cancel" semantics: this created a severe UX bug.
		if looksLikeCancelPrompt(body.Prompt) {
			err := errors.New("cancel prompt is not supported; use task_cancel_submit instead")
			ops.AppendActionAuditLog(ctx, taskID, "task_continue_submit", map[string]any{"task_id": taskID, "session_key": key, "prompt": body.Prompt}, err)
			return nil, err
		}
	}
	res, err := ops.ContinueSession(ctx, key, body)
	ops.AppendActionAuditLog(ctx, taskID, "task_continue_submit", map[string]any{"task_id": taskID, "session_key": key, "prompt": body.Prompt}, err)
	if err != nil {
		return nil, err
	}
	if res.Queue != nil {
		return map[string]any{
			"queued":           res.Queue.Queued,
			"queue_id":         res.Queue.QueueID,
			"position":         res.Queue.Position,
			"existing_task_id": res.Queue.ExistingTaskID,
			"existing_status":  res.Queue.ExistingStatus,
		}, nil
	}
	if res.Task != nil {
		return map[string]any{"task": *res.Task}, nil
	}
	return map[string]any{"ok": true}, nil
}
