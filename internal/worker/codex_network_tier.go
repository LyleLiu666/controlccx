package worker

import (
	"strings"

	"controlccx/internal/tasks"
)

func codexSandboxInputForTask(task tasks.Task) string {
	if raw := strings.TrimSpace(task.CodexSandbox); raw != "" {
		return raw
	}
	if !shouldApplyCodexTierDefaults(task) {
		return ""
	}
	switch task.NetworkTier {
	case tasks.NetworkTierOff:
		return "read-only"
	case tasks.NetworkTierWebReadonly:
		return "workspace-write"
	case tasks.NetworkTierExecNet:
		return "danger-full-access"
	default:
		return ""
	}
}

func codexSearchEnabledForTask(task tasks.Task) bool {
	// Network off is a hard stop for Codex web search.
	if task.NetworkTier == tasks.NetworkTierOff {
		return false
	}
	if task.CodexSearch {
		return true
	}
	if !shouldApplyCodexTierDefaults(task) {
		return false
	}
	switch task.NetworkTier {
	case tasks.NetworkTierWebReadonly, tasks.NetworkTierExecNet:
		return true
	default:
		return false
	}
}

func shouldApplyCodexTierDefaults(task tasks.Task) bool {
	if strings.TrimSpace(string(task.NetworkTier)) == "" {
		return false
	}
	return strings.TrimSpace(task.CodexSandbox) == "" && !task.CodexSearch
}
