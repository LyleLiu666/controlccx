package tasks

import (
	"fmt"
	"strings"
)

type NetworkTier string

const (
	NetworkTierOff         NetworkTier = "off"
	NetworkTierWebReadonly NetworkTier = "web_readonly"
	NetworkTierExecNet     NetworkTier = "exec_net"
)

func NormalizeNetworkTier(raw string) NetworkTier {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off", "no-network", "none":
		return NetworkTierOff
	case "web_readonly", "web-readonly", "search-browse":
		return NetworkTierWebReadonly
	case "exec_net", "exec-net", "full-network", "unsafe", "danger-full-access":
		return NetworkTierExecNet
	default:
		return ""
	}
}

func resolveCreateNetworkTier(in CreateTaskInput) (NetworkTier, error) {
	raw := strings.TrimSpace(string(in.NetworkTier))
	if raw == "" {
		return deriveNetworkTierFromLegacy(in.UnsafeAutomation, in.SafetyPreset, in.CodexSearch, in.ClaudeSandbox, in.ClaudeWebFetchDomains), nil
	}
	if tier := NormalizeNetworkTier(raw); tier != "" {
		return tier, nil
	}
	return "", fmt.Errorf("tasks: invalid network_tier %q", raw)
}

func resolveTaskNetworkTier(raw string, t Task) NetworkTier {
	if tier := NormalizeNetworkTier(raw); tier != "" {
		return tier
	}
	return deriveNetworkTierFromLegacy(t.UnsafeAutomation, t.SafetyPreset, t.CodexSearch, t.ClaudeSandbox, t.ClaudeWebFetchDomains)
}

func deriveNetworkTierFromLegacy(unsafe bool, safetyPreset string, codexSearch bool, claudeSandbox bool, claudeDomains []string) NetworkTier {
	preset := strings.ToLower(strings.TrimSpace(safetyPreset))
	if unsafe || strings.Contains(preset, "unsafe") || strings.Contains(preset, "danger-full-access") {
		return NetworkTierExecNet
	}
	if strings.Contains(preset, "no-network") || preset == "off" {
		return NetworkTierOff
	}
	if codexSearch || len(normalizeDomains(claudeDomains)) > 0 {
		return NetworkTierWebReadonly
	}
	if strings.Contains(preset, "search-browse") || strings.Contains(preset, "read-only") || strings.Contains(preset, "workspace-write") {
		return NetworkTierWebReadonly
	}
	if !claudeSandbox && preset != "" {
		return NetworkTierExecNet
	}
	// Project-level default from the 30-change plan is web_readonly.
	return NetworkTierWebReadonly
}
