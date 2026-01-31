package runsafe

import (
	"context"
	"testing"

	"controlccx/internal/tasks"
)

func TestApplyAutopilot_Codex(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "帮我搜索一下 Claude Code sandbox 的文档",
		WorkDir:    ".",
	}
	out, res := ApplyAutopilot(context.Background(), in, ApplyOptions{Driver: tasks.WorkerCodex})
	if !res.Applied {
		t.Fatalf("expected applied")
	}
	if out.TaskIntent != "search-browse" {
		t.Fatalf("task_intent=%q, want %q", out.TaskIntent, "search-browse")
	}
	if out.SafetyPreset != "search-browse" {
		t.Fatalf("safety_preset=%q, want %q", out.SafetyPreset, "search-browse")
	}
	if out.CodexSandbox != "workspace-write" {
		t.Fatalf("codex_sandbox=%q, want %q", out.CodexSandbox, "workspace-write")
	}
	if out.CodexApprovalPolicy != "never" {
		t.Fatalf("codex_approval_policy=%q, want %q", out.CodexApprovalPolicy, "never")
	}
	if !out.CodexSearch {
		t.Fatalf("codex_search=%v, want true", out.CodexSearch)
	}
}

func TestApplyAutopilot_Claude_Analyze(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "请总结这段代码的主要逻辑，并指出可能的 bug",
		WorkDir:    ".",
	}
	out, res := ApplyAutopilot(context.Background(), in, ApplyOptions{Driver: tasks.WorkerClaudeCode})
	if !res.Applied {
		t.Fatalf("expected applied")
	}
	if out.TaskIntent != "analyze" {
		t.Fatalf("task_intent=%q, want %q", out.TaskIntent, "analyze")
	}
	if out.SafetyPreset != "no-network" {
		t.Fatalf("safety_preset=%q, want %q", out.SafetyPreset, "no-network")
	}
	if !out.ClaudeSandbox {
		t.Fatalf("claude_sandbox=%v, want true", out.ClaudeSandbox)
	}
	if out.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want false", out.UnsafeAutomation)
	}
}

func TestApplyAutopilot_Claude_Install_RequiresUnlock(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "npm install 并运行测试",
		WorkDir:    ".",
	}

	out, res := ApplyAutopilot(context.Background(), in, ApplyOptions{
		Driver:   tasks.WorkerClaudeCode,
		Envelope: EnvelopeDefault,
	})
	if !res.Applied {
		t.Fatalf("expected applied")
	}
	if out.TaskIntent != "install" {
		t.Fatalf("task_intent=%q, want %q", out.TaskIntent, "install")
	}
	if out.SafetyPreset != "no-network" {
		t.Fatalf("safety_preset=%q, want %q", out.SafetyPreset, "no-network")
	}
	if out.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want false", out.UnsafeAutomation)
	}

	unlocked, _ := ApplyAutopilot(context.Background(), in, ApplyOptions{
		Driver:   tasks.WorkerClaudeCode,
		Envelope: EnvelopeInstallEnabled,
	})
	if unlocked.SafetyPreset != "unsafe" || !unlocked.UnsafeAutomation {
		t.Fatalf("unlock preset/unsafe=%q/%v, want unsafe/true", unlocked.SafetyPreset, unlocked.UnsafeAutomation)
	}
}
