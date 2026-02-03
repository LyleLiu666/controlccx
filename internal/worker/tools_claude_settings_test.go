package worker

import (
	"encoding/json"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestBuildToolCommand_Claude_PermissionMode_IsPassed(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:           tasks.WorkerClaudeCode,
		Mode:                 tasks.ModeNew,
		Prompt:               "hi",
		WorkDir:              ".",
		ClaudePermissionMode: "plan",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	if !hasArg(tool.Args, "--permission-mode") || !hasArg(tool.Args, "plan") {
		t.Fatalf("args=%v, expected --permission-mode plan", tool.Args)
	}
}

func TestBuildToolCommand_Claude_NoNetwork_DeniesWebFetchAndCurl(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:    tasks.WorkerClaudeCode,
		Mode:          tasks.ModeNew,
		Prompt:        "hi",
		WorkDir:       ".",
		SafetyPreset:  "claude:sandboxed-no-network",
		TaskIntent:    "analyze",
		ClaudeSandbox: true,
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	settings := mustExtractClaudeSettings(t, tool.Args)

	deny := stringsFromAny(settings["permissions"], "deny")
	if !contains(deny, "WebFetch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebFetch", deny)
	}
	if !contains(deny, "Bash(curl *)") {
		t.Fatalf("settings.permissions.deny=%v, expected Bash(curl *)", deny)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Fatalf("settings.sandbox.allowUnsandboxedCommands=%v, expected false", sandbox["allowUnsandboxedCommands"])
	}
}

func TestBuildToolCommand_Claude_SearchBrowse_AllowsWebFetchAndDeniesCurlWget(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:            tasks.WorkerClaudeCode,
		Mode:                  tasks.ModeNew,
		Prompt:                "hi",
		WorkDir:               ".",
		SafetyPreset:          "claude:sandboxed-search-browse",
		TaskIntent:            "search-browse",
		ClaudeSandbox:         true,
		ClaudeWebFetchDomains: []string{"docs.claude.com"},
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	settings := mustExtractClaudeSettings(t, tool.Args)

	deny := stringsFromAny(settings["permissions"], "deny")
	if !contains(deny, "Bash(curl *)") || !contains(deny, "Bash(wget *)") {
		t.Fatalf("settings.permissions.deny=%v, expected curl+wget denied", deny)
	}
	if contains(deny, "WebFetch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebFetch not denied", deny)
	}

	allow := stringsFromAny(settings["permissions"], "allow")
	if !contains(allow, "WebFetch(domain:docs.claude.com)") {
		t.Fatalf("settings.permissions.allow=%v, expected WebFetch(domain:docs.claude.com) allowed", allow)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Fatalf("settings.sandbox.allowUnsandboxedCommands=%v, expected false", sandbox["allowUnsandboxedCommands"])
	}
}

func TestBuildToolCommand_Claude_Unsafe_DoesNotDenyCurlWget(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:       tasks.WorkerClaudeCode,
		Mode:             tasks.ModeNew,
		Prompt:           "hi",
		WorkDir:          ".",
		SafetyPreset:     "unsafe",
		TaskIntent:       "install",
		UnsafeAutomation: true,
		ClaudeSandbox:    true,
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	settings := mustExtractClaudeSettings(t, tool.Args)

	deny := stringsFromAny(settings["permissions"], "deny")
	if contains(deny, "Bash(curl *)") || contains(deny, "Bash(wget *)") {
		t.Fatalf("settings.permissions.deny=%v, expected curl+wget not denied", deny)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["allowUnsandboxedCommands"] != true {
		t.Fatalf("settings.sandbox.allowUnsandboxedCommands=%v, expected true", sandbox["allowUnsandboxedCommands"])
	}
}

func mustExtractClaudeSettings(t *testing.T, args []string) map[string]any {
	t.Helper()
	i := indexOf(args, "--settings")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("args=%v, expected --settings <json>", args)
	}
	raw := args[i+1]
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse settings json: %v; raw=%q", err, raw)
	}
	return out
}

func stringsFromAny(obj any, key string) []string {
	m, ok := obj.(map[string]any)
	if !ok {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		s, ok := it.(string)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

func contains(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}
