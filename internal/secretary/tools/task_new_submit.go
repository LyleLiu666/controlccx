package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type taskNewSubmitTool struct{}

func (taskNewSubmitTool) Name() string { return "task_new_submit" }

func (taskNewSubmitTool) DescriptionZH() string {
	return "创建全新任务。参数：worker_type（必填，worker_type 仅允许 claude-code | codex | exec）、prompt（必填）、workdir（必填）；可选：conversation_id、workdir_strategy、worktree_untracked 及安全参数。worker 选择建议：简单且追求速度 -> claude-code；严肃/生产级迭代 -> codex；不确定则先问。自动推荐不使用 exec，exec 仅在用户明确要求 shell/脚本执行时使用。"
}

func (taskNewSubmitTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}

	workerType := strings.TrimSpace(call.Fields["worker_type"])
	if workerType == "" {
		return nil, errors.New("worker_type is required")
	}
	prompt := strings.TrimSpace(call.Fields["prompt"])
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	workdir := strings.TrimSpace(call.Fields["workdir"])
	if workdir == "" {
		return nil, errors.New("workdir is required")
	}

	in := tasks.CreateTaskInput{
		WorkerType:            tasks.WorkerType(workerType),
		Mode:                  tasks.ModeNew,
		ConversationID:        strings.TrimSpace(call.Fields["conversation_id"]),
		WorkDirStrategy:       strings.TrimSpace(call.Fields["workdir_strategy"]),
		WorktreeUntracked:     strings.TrimSpace(call.Fields["worktree_untracked"]),
		UnsafeAutomation:      parseBool(call.Fields["unsafe_automation"]),
		SafetyEnvelope:        strings.TrimSpace(call.Fields["safety_envelope"]),
		SafetyPreset:          strings.TrimSpace(call.Fields["safety_preset"]),
		TaskIntent:            strings.TrimSpace(call.Fields["task_intent"]),
		CodexSandbox:          strings.TrimSpace(call.Fields["codex_sandbox"]),
		CodexApprovalPolicy:   strings.TrimSpace(call.Fields["codex_approval_policy"]),
		CodexSearch:           parseBool(call.Fields["codex_search"]),
		ClaudePermissionMode:  strings.TrimSpace(call.Fields["claude_permission_mode"]),
		ClaudeSandbox:         parseBool(call.Fields["claude_sandbox"]),
		ClaudeWebFetchDomains: parseStringSliceCSV(call.Fields["claude_webfetch_domains"]),
		Prompt:                prompt,
		WorkDir:               workdir,
	}

	t, err := ops.CreateTask(ctx, in)
	if err != nil {
		return nil, err
	}
	ops.AppendActionAuditLog(ctx, t.ID, "task_new_submit", map[string]any{
		"task_id":          t.ID,
		"worker_type":      workerType,
		"workdir":          workdir,
		"prompt":           prompt,
		"conversation_id":  strings.TrimSpace(call.Fields["conversation_id"]),
		"workdir_strategy": strings.TrimSpace(call.Fields["workdir_strategy"]),
	}, nil)
	return map[string]any{"task": t}, nil
}
