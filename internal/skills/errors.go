package skills

import (
	"fmt"
	"strings"
)

const (
	errPrefixMultiSkills      = "MULTI_SKILLS|"
	errPrefixTargetExists     = "TARGET_EXISTS|"
	errPrefixToolNotInstalled = "TOOL_NOT_INSTALLED|"
)

func errMultiSkills(msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "multiple skills detected; selection required"
	}
	return fmt.Errorf("%s%s", errPrefixMultiSkills, msg)
}

func errTargetExists(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%starget exists", errPrefixTargetExists)
	}
	return fmt.Errorf("%s%s", errPrefixTargetExists, path)
}

func errToolNotInstalled(tool string) error {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		tool = "unknown"
	}
	return fmt.Errorf("%s%s", errPrefixToolNotInstalled, tool)
}

func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}
