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
	return "创建全新任务。参数：worker_type（必填，worker_type 仅允许 claude-code | codex | exec）、prompt（必填）、workdir（必填）；可选：conversation_id、workdir_strategy、worktree_untracked 及安全参数。worker_type 语义：claude-code=Claude Code 代理执行；codex=Codex 代理执行；exec=在本机 workdir 直接执行你提供的 shell（bash）命令（原样交给 Unix 的 sh -lc / Windows 的 cmd.exe /c；不会做自然语言转译，prompt 必须是可直接执行的命令字符串；由 worker 进程执行，不是秘书自身执行）。选择建议：简单且追求速度 -> claude-code；严肃/生产级迭代 -> codex；不确定则先问。自动推荐不使用 exec，exec 仅在用户明确要求执行具体 shell 命令时使用。"
}

func (taskNewSubmitTool) Params() []string {
	base := []string{
		"worker_type",
		"prompt",
		"workdir",
		"conversation_id",
		"workdir_strategy",
		"worktree_untracked",
	}
	return append(base, RunOptsParams...)
}

func (taskNewSubmitTool) Required() []string { return []string{"worker_type", "prompt", "workdir"} }

func (taskNewSubmitTool) AnyOfRequired() [][]string { return nil }

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
		NetworkTier:           tasks.NetworkTier(strings.TrimSpace(call.Fields["network_tier"])),
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
