package runsafe

import (
	"context"
	"strings"
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
	if out.CodexApprovalPolicy != "untrusted" {
		t.Fatalf("codex_approval_policy=%q, want %q", out.CodexApprovalPolicy, "untrusted")
	}
	if !out.CodexSearch {
		t.Fatalf("codex_search=%v, want true", out.CodexSearch)
	}
	if out.NetworkTier != tasks.NetworkTierWebReadonly {
		t.Fatalf("network_tier=%q, want %q", out.NetworkTier, tasks.NetworkTierWebReadonly)
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
	if out.SafetyPreset != "search-browse" {
		t.Fatalf("safety_preset=%q, want %q", out.SafetyPreset, "search-browse")
	}
	if out.ClaudePermissionMode != "acceptEdits" {
		t.Fatalf("claude_permission_mode=%q, want %q", out.ClaudePermissionMode, "acceptEdits")
	}
	if !out.ClaudeSandbox {
		t.Fatalf("claude_sandbox=%v, want true", out.ClaudeSandbox)
	}
	if out.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want false", out.UnsafeAutomation)
	}
	if out.NetworkTier != tasks.NetworkTierWebReadonly {
		t.Fatalf("network_tier=%q, want %q", out.NetworkTier, tasks.NetworkTierWebReadonly)
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
	if out.SafetyPreset != "search-browse" {
		t.Fatalf("safety_preset=%q, want %q", out.SafetyPreset, "search-browse")
	}
	if out.ClaudePermissionMode != "acceptEdits" {
		t.Fatalf("claude_permission_mode=%q, want %q", out.ClaudePermissionMode, "acceptEdits")
	}
	if out.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want false", out.UnsafeAutomation)
	}
	if out.NetworkTier != tasks.NetworkTierWebReadonly {
		t.Fatalf("network_tier=%q, want %q", out.NetworkTier, tasks.NetworkTierWebReadonly)
	}

	unlocked, _ := ApplyAutopilot(context.Background(), in, ApplyOptions{
		Driver:   tasks.WorkerClaudeCode,
		Envelope: EnvelopeInstallEnabled,
	})
	if unlocked.SafetyPreset != "unsafe" || !unlocked.UnsafeAutomation {
		t.Fatalf("unlock preset/unsafe=%q/%v, want unsafe/true", unlocked.SafetyPreset, unlocked.UnsafeAutomation)
	}
	if unlocked.NetworkTier != tasks.NetworkTierExecNet {
		t.Fatalf("unlock network_tier=%q, want %q", unlocked.NetworkTier, tasks.NetworkTierExecNet)
	}
	if unlocked.ClaudeSandbox {
		t.Fatalf("unlock claude_sandbox=%v, want false", unlocked.ClaudeSandbox)
	}
	if unlocked.ClaudePermissionMode != "" {
		t.Fatalf("unlock claude_permission_mode=%q, want empty", unlocked.ClaudePermissionMode)
	}
}

func TestApplyAutopilot_WebReadonlyNetworkTierStillAppliesDefaults(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType:  tasks.WorkerClaudeCode,
		Mode:        tasks.ModeNew,
		Prompt:      "请总结这个模块",
		WorkDir:     ".",
		NetworkTier: tasks.NetworkTierWebReadonly,
	}
	out, res := ApplyAutopilot(context.Background(), in, ApplyOptions{Driver: tasks.WorkerClaudeCode})
	if !res.Applied {
		t.Fatalf("expected autopilot to apply when network_tier=web_readonly only")
	}
	if out.SafetyPreset == "" {
		t.Fatalf("expected autopilot to set safety_preset")
	}
}

func TestApplyAutopilot_OffNetworkTierSkipsAutopilot(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType:  tasks.WorkerClaudeCode,
		Mode:        tasks.ModeNew,
		Prompt:      "请总结这个模块",
		WorkDir:     ".",
		NetworkTier: tasks.NetworkTierOff,
	}
	out, res := ApplyAutopilot(context.Background(), in, ApplyOptions{Driver: tasks.WorkerClaudeCode})
	if res.Applied {
		t.Fatalf("expected autopilot not to apply for explicit off tier")
	}
	if out.NetworkTier != tasks.NetworkTierOff {
		t.Fatalf("network_tier=%q, want %q", out.NetworkTier, tasks.NetworkTierOff)
	}
}

func TestFormatAuditLog_IncludesEnvelopeAndUnsafeAutomation(t *testing.T) {
	in := tasks.CreateTaskInput{
		WorkerType:       tasks.WorkerClaudeCode,
		TaskIntent:       "code",
		SafetyPreset:     "search-browse",
		SafetyEnvelope:   "install-enabled",
		UnsafeAutomation: false,
	}
	decision := Decision{Intent: IntentCode}
	out := FormatAuditLog(tasks.WorkerClaudeCode, decision, in, true)
	if !strings.Contains(out, "unsafe_automation=false") {
		t.Fatalf("missing unsafe_automation in %q", out)
	}
	if !strings.Contains(out, "envelope=install-enabled") {
		t.Fatalf("missing envelope in %q", out)
	}

	in.UnsafeAutomation = true
	in.SafetyPreset = "unsafe"
	out = FormatAuditLog(tasks.WorkerClaudeCode, decision, in, true)
	if !strings.Contains(out, "unsafe_automation=true") {
		t.Fatalf("missing unsafe_automation=true in %q", out)
	}
}
