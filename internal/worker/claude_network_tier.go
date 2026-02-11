package worker

import (
	"strings"

	"controlccx/internal/tasks"
)

func shouldApplyClaudeTierDefaults(task tasks.Task) bool {
	if strings.TrimSpace(string(task.NetworkTier)) == "" {
		return false
	}
	if strings.TrimSpace(task.SafetyPreset) != "" || strings.TrimSpace(task.TaskIntent) != "" {
		return false
	}
	if task.ClaudeSandbox || len(task.ClaudeWebFetchDomains) > 0 {
		return false
	}
	return true
}

func claudeNoNetworkForTask(task tasks.Task, preset string, tierDefaults bool) bool {
	if strings.Contains(preset, "no-network") || preset == "off" {
		return true
	}
	return tierDefaults && task.NetworkTier == tasks.NetworkTierOff
}

func claudeAllowCurlWgetForTask(task tasks.Task, preset string, tierDefaults bool, noNetwork bool) bool {
	if noNetwork {
		return false
	}
	if tierDefaults {
		return task.NetworkTier == tasks.NetworkTierExecNet
	}
	taskIntent := strings.ToLower(strings.TrimSpace(task.TaskIntent))
	return taskIntent == "search-browse" || (taskIntent == "" && strings.Contains(preset, "search-browse"))
}

func claudeSandboxEnabledForTask(task tasks.Task, tierDefaults bool) bool {
	_ = tierDefaults
	return task.ClaudeSandbox
}
