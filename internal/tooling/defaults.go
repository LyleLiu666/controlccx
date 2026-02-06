package tooling

import (
	"strings"

	"controlccx/internal/config"
)

func DefaultsFromConfig(cfg config.Config) []Tool {
	claude := strings.TrimSpace(cfg.Paths.Claude)
	if claude == "" {
		claude = "claude"
	}
	codex := strings.TrimSpace(cfg.Paths.Codex)
	if codex == "" {
		codex = "codex"
	}
	return []Tool{
		{ID: "claude-code", Driver: DriverClaudeCode, Command: claude},
		{ID: "codex", Driver: DriverCodex, Command: codex},
	}
}
