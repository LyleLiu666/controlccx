package tools

import (
	"strings"

	"controlccx/internal/taskops"
)

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
		CodexSandbox:          strings.TrimSpace(fields["codex_sandbox"]),
		CodexApprovalPolicy:   strings.TrimSpace(fields["codex_approval_policy"]),
		CodexSearch:           parseBool(fields["codex_search"]),
		ClaudePermissionMode:  strings.TrimSpace(fields["claude_permission_mode"]),
		ClaudeSandbox:         parseBool(fields["claude_sandbox"]),
		ClaudeWebFetchDomains: parseStringSliceCSV(fields["claude_webfetch_domains"]),
	}
}
