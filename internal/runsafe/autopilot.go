package runsafe

import (
	"context"
	"fmt"
	"strings"

	"controlccx/internal/tasks"
)

type ApplyOptions struct {
	Driver   tasks.WorkerType
	Envelope SafetyEnvelope
	Classify ClassifyOptions
}

type ApplyResult struct {
	Applied  bool
	Decision Decision
}

func ApplyAutopilot(ctx context.Context, in tasks.CreateTaskInput, opts ApplyOptions) (tasks.CreateTaskInput, ApplyResult) {
	driver := opts.Driver
	if driver != tasks.WorkerClaudeCode && driver != tasks.WorkerCodex {
		return in, ApplyResult{Applied: false}
	}
	if hasExplicitSafetyOptions(in) {
		return in, ApplyResult{Applied: false}
	}

	env := opts.Envelope
	if strings.TrimSpace(string(env)) == "" {
		env = EnvelopeDefault
	}

	decision := ClassifyPrompt(ctx, in.Prompt, opts.Classify)

	switch driver {
	case tasks.WorkerCodex:
		applyCodexAutopilot(&in, decision, env)
	case tasks.WorkerClaudeCode:
		applyClaudeAutopilot(&in, decision, env)
	}

	return in, ApplyResult{Applied: true, Decision: decision}
}

func hasExplicitSafetyOptions(in tasks.CreateTaskInput) bool {
	if in.UnsafeAutomation {
		return true
	}
	if strings.TrimSpace(in.SafetyPreset) != "" || strings.TrimSpace(in.TaskIntent) != "" {
		return true
	}
	if strings.TrimSpace(in.CodexSandbox) != "" || strings.TrimSpace(in.CodexApprovalPolicy) != "" || in.CodexSearch {
		return true
	}
	if strings.TrimSpace(in.ClaudePermissionMode) != "" || in.ClaudeSandbox || len(in.ClaudeWebFetchDomains) > 0 {
		return true
	}
	return false
}

func applyCodexAutopilot(in *tasks.CreateTaskInput, decision Decision, env SafetyEnvelope) {
	intent := string(decision.Intent)
	in.TaskIntent = intent
	in.CodexApprovalPolicy = "never"

	switch decision.Intent {
	case IntentAnalyze:
		in.SafetyPreset = "read-only"
		in.CodexSandbox = "read-only"
		in.CodexSearch = false
	case IntentSearchBrowse:
		in.SafetyPreset = "search-browse"
		in.CodexSandbox = "workspace-write"
		in.CodexSearch = true
	case IntentInstall:
		// Install is considered high-risk. Only allow the more permissive sandbox when explicitly unlocked.
		if env == EnvelopeInstallEnabled {
			in.SafetyPreset = "danger-full-access"
			in.CodexSandbox = "danger-full-access"
			in.CodexSearch = false
		} else {
			in.SafetyPreset = "workspace-write"
			in.CodexSandbox = "workspace-write"
			in.CodexSearch = false
		}
	default:
		in.SafetyPreset = "workspace-write"
		in.CodexSandbox = "workspace-write"
		in.CodexSearch = false
	}
}

func applyClaudeAutopilot(in *tasks.CreateTaskInput, decision Decision, env SafetyEnvelope) {
	intent := string(decision.Intent)
	in.TaskIntent = intent
	// Safe-by-default: enable Claude bash sandboxing unless the run explicitly opts into unsafe.
	in.ClaudeSandbox = true

	switch decision.Intent {
	case IntentSearchBrowse:
		in.SafetyPreset = "search-browse"
	case IntentInstall:
		// Install is considered high-risk. Only bypass permissions when explicitly unlocked.
		if env == EnvelopeInstallEnabled {
			in.SafetyPreset = "unsafe"
			in.UnsafeAutomation = true
			// Unsafe installs often require arbitrary network access; disable bash sandbox so tools like
			// pip/curl can reach the public internet.
			in.ClaudeSandbox = false
		} else {
			// Default to allowing WebFetch so the agent can read docs/instructions before asking for unlock.
			in.SafetyPreset = "search-browse"
		}
	case IntentAnalyze:
		// Default to allowing WebFetch; most analysis tasks benefit from reading docs.
		in.SafetyPreset = "search-browse"
	default:
		// "code" defaults to search-browse; WebFetch is low-risk compared to downloading/executing scripts.
		in.SafetyPreset = "search-browse"
	}

	// Non-interactive Claude runs cannot respond to interactive approval prompts.
	// Accept file edits by default unless the run explicitly bypasses all permissions.
	if !in.UnsafeAutomation && strings.TrimSpace(in.SafetyPreset) != "unsafe" {
		in.ClaudePermissionMode = "acceptEdits"
	}
}

func FormatAuditLog(driver tasks.WorkerType, decision Decision, in tasks.CreateTaskInput, applied bool) string {
	if !applied {
		return ""
	}
	intent := strings.TrimSpace(in.TaskIntent)
	preset := strings.TrimSpace(in.SafetyPreset)
	envelope := strings.TrimSpace(in.SafetyEnvelope)
	if intent == "" {
		intent = string(decision.Intent)
	}
	parts := []string{
		"safety.autopilot",
		fmt.Sprintf("driver=%s", strings.TrimSpace(string(driver))),
		fmt.Sprintf("unsafe_automation=%t", in.UnsafeAutomation),
		fmt.Sprintf("intent=%s", intent),
		fmt.Sprintf("preset=%s", preset),
	}
	if envelope != "" {
		parts = append(parts, fmt.Sprintf("envelope=%s", envelope))
	}
	if len(decision.Signals) > 0 {
		parts = append(parts, "signals="+strings.Join(decision.Signals, ","))
	}
	if strings.TrimSpace(decision.Reason) != "" {
		parts = append(parts, "reason="+strings.TrimSpace(decision.Reason))
	}
	return strings.Join(parts, " ")
}
