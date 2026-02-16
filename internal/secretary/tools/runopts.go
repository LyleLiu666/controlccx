package tools

import (
	"strings"

	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

var RunOptsParams = []string{
	"unsafe_automation",
	"safety_envelope",
	"safety_preset",
	"task_intent",
	"network_tier",
	"codex_sandbox",
	"codex_approval_policy",
	"codex_search",
	"claude_permission_mode",
	"claude_sandbox",
	"claude_webfetch_domains",
}

func runOptionsFromFields(fields map[string]string) taskops.RunOptions {
	if fields == nil {
		fields = map[string]string{}
	}
	return taskops.RunOptions{
		Prompt:                strings.TrimSpace(fields["prompt"]),
		UnsafeAutomation:      parseBool(fields["unsafe_automation"]),
		SafetyEnvelope:        strings.TrimSpace(fields["safety_envelope"]),
		SafetyPreset:          strings.TrimSpace(fields["safety_preset"]),
		TaskIntent:            strings.TrimSpace(fields["task_intent"]),
		NetworkTier:           tasks.NetworkTier(strings.TrimSpace(fields["network_tier"])),
		CodexSandbox:          strings.TrimSpace(fields["codex_sandbox"]),
		CodexApprovalPolicy:   strings.TrimSpace(fields["codex_approval_policy"]),
		CodexSearch:           parseBool(fields["codex_search"]),
		ClaudePermissionMode:  strings.TrimSpace(fields["claude_permission_mode"]),
		ClaudeSandbox:         parseBool(fields["claude_sandbox"]),
		ClaudeWebFetchDomains: parseStringSliceCSV(fields["claude_webfetch_domains"]),
	}
}
