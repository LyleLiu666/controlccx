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
	if !contains(deny, "WebSearch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebSearch", deny)
	}
	if !contains(deny, "Bash(curl *)") {
		t.Fatalf("settings.permissions.deny=%v, expected Bash(curl *)", deny)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Fatalf("settings.sandbox.allowUnsandboxedCommands=%v, expected false", sandbox["allowUnsandboxedCommands"])
	}
}

func TestBuildToolCommand_Claude_SearchBrowse_AllowsWebFetchAndAllowsCurlWget(t *testing.T) {
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
	if contains(deny, "Bash(curl *)") || contains(deny, "Bash(wget *)") {
		t.Fatalf("settings.permissions.deny=%v, expected curl+wget not denied", deny)
	}
	if contains(deny, "WebFetch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebFetch not denied", deny)
	}
	if contains(deny, "WebSearch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebSearch not denied", deny)
	}

	allow := stringsFromAny(settings["permissions"], "allow")
	if !contains(allow, "WebFetch(domain:docs.claude.com)") {
		t.Fatalf("settings.permissions.allow=%v, expected WebFetch(domain:docs.claude.com) allowed", allow)
	}
	if !contains(allow, "WebSearch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebSearch allowed", allow)
	}

	sandbox := settings["sandbox"].(map[string]any)
	if sandbox["allowUnsandboxedCommands"] != false {
		t.Fatalf("settings.sandbox.allowUnsandboxedCommands=%v, expected false", sandbox["allowUnsandboxedCommands"])
	}
}

func TestBuildToolCommand_Claude_TierOnlyWebReadonly_MapsToBrowseSafeDefaults(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:  tasks.WorkerClaudeCode,
		Mode:        tasks.ModeNew,
		Prompt:      "hi",
		WorkDir:     ".",
		NetworkTier: tasks.NetworkTierWebReadonly,
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	settings := mustExtractClaudeSettings(t, tool.Args)
	deny := stringsFromAny(settings["permissions"], "deny")
	if !contains(deny, "Bash(curl *)") || !contains(deny, "Bash(wget *)") {
		t.Fatalf("settings.permissions.deny=%v, expected curl+wget denied for web_readonly", deny)
	}
	allow := stringsFromAny(settings["permissions"], "allow")
	if !contains(allow, "WebFetch") || !contains(allow, "WebSearch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebFetch+WebSearch allowed", allow)
	}
}

func TestBuildToolCommand_Claude_TierOnlyOff_DeniesWebTools(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:  tasks.WorkerClaudeCode,
		Mode:        tasks.ModeNew,
		Prompt:      "hi",
		WorkDir:     ".",
		NetworkTier: tasks.NetworkTierOff,
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	settings := mustExtractClaudeSettings(t, tool.Args)
	deny := stringsFromAny(settings["permissions"], "deny")
	if !contains(deny, "WebFetch") || !contains(deny, "WebSearch") {
		t.Fatalf("settings.permissions.deny=%v, expected WebFetch+WebSearch denied", deny)
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
	if sandbox["enabled"] != false {
		t.Fatalf("settings.sandbox.enabled=%v, expected false", sandbox["enabled"])
	}
}

func TestBuildToolCommand_Claude_SearchBrowse_Default_AllowsWebFetchAndWebSearch(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:           tasks.WorkerClaudeCode,
		Mode:                 tasks.ModeNew,
		Prompt:               "hi",
		WorkDir:              ".",
		SafetyPreset:         "search-browse",
		TaskIntent:           "search-browse",
		ClaudeSandbox:        true,
		ClaudePermissionMode: "acceptEdits",
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

	allow := stringsFromAny(settings["permissions"], "allow")
	if !contains(allow, "WebFetch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebFetch allowed", allow)
	}
	if !contains(allow, "WebSearch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebSearch allowed", allow)
	}
}

func TestBuildToolCommand_Claude_CodeIntentWithSearchBrowsePreset_DeniesCurlWget(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:           tasks.WorkerClaudeCode,
		Mode:                 tasks.ModeNew,
		Prompt:               "hi",
		WorkDir:              ".",
		SafetyPreset:         "search-browse",
		TaskIntent:           "code",
		ClaudeSandbox:        true,
		ClaudePermissionMode: "acceptEdits",
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

	allow := stringsFromAny(settings["permissions"], "allow")
	if !contains(allow, "WebFetch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebFetch allowed", allow)
	}
	if !contains(allow, "WebSearch") {
		t.Fatalf("settings.permissions.allow=%v, expected WebSearch allowed", allow)
	}
}

func TestBuildToolCommand_Claude_StreamJSONProtocolFlags_ArePresent(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:           tasks.WorkerClaudeCode,
		Mode:                 tasks.ModeNew,
		Prompt:               "hi",
		WorkDir:              ".",
		ClaudePermissionMode: "acceptEdits",
		SafetyPreset:         "search-browse",
		TaskIntent:           "search-browse",
		ClaudeSandbox:        true,
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	// Ensure non-interactive JSON protocol is configured; otherwise Claude permission prompts
	// may block runs with no way to approve.
	for _, want := range []string{
		"-p",
		"--permission-prompt-tool=stdio",
		"--verbose",
		"--output-format=stream-json",
		"--input-format=stream-json",
		"--include-partial-messages",
		"--disallowedTools=AskUserQuestion",
	} {
		if !hasArg(tool.Args, want) {
			t.Fatalf("args=%v, expected %q", tool.Args, want)
		}
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

func TestIsToolAutoAllowed_SearchBrowse_AllowsWebSearchAndWebFetch(t *testing.T) {
	task := tasks.Task{
		SafetyPreset:  "search-browse",
		TaskIntent:    "search-browse",
		ClaudeSandbox: true,
	}
	if !isToolAutoAllowed(task, "WebSearch", nil) {
		t.Fatal("WebSearch should be auto-allowed for search-browse")
	}
	if !isToolAutoAllowed(task, "WebFetch", nil) {
		t.Fatal("WebFetch should be auto-allowed for search-browse")
	}
	if !isToolAutoAllowed(task, "websearch", nil) {
		t.Fatal("websearch (lowercase) should be auto-allowed")
	}
}

func TestIsToolAutoAllowed_TierOnlyWebReadonly_RequiresApproval(t *testing.T) {
	task := tasks.Task{
		NetworkTier: tasks.NetworkTierWebReadonly,
	}
	if isToolAutoAllowed(task, "WebSearch", nil) {
		t.Fatal("WebSearch should require approval for tier-only web_readonly")
	}
	if isToolAutoAllowed(task, "WebFetch", nil) {
		t.Fatal("WebFetch should require approval for tier-only web_readonly")
	}
}

func TestIsToolAutoAllowed_NoNetwork_DeniesWebSearch(t *testing.T) {
	task := tasks.Task{
		SafetyPreset:  "no-network",
		TaskIntent:    "analyze",
		ClaudeSandbox: true,
	}
	if isToolAutoAllowed(task, "WebSearch", nil) {
		t.Fatal("WebSearch should NOT be auto-allowed for no-network")
	}
	if isToolAutoAllowed(task, "WebFetch", nil) {
		t.Fatal("WebFetch should NOT be auto-allowed for no-network")
	}
}

func TestIsToolAutoAllowed_NoSettings_ReturnsFalse(t *testing.T) {
	task := tasks.Task{}
	if isToolAutoAllowed(task, "WebSearch", nil) {
		t.Fatal("WebSearch should NOT be auto-allowed when no settings")
	}
}

func TestIsToolAutoAllowed_NonWebTool_ReturnsFalse(t *testing.T) {
	task := tasks.Task{
		SafetyPreset:  "search-browse",
		TaskIntent:    "search-browse",
		ClaudeSandbox: true,
	}
	if isToolAutoAllowed(task, "Bash", nil) {
		t.Fatal("Bash should NOT be auto-allowed")
	}
	if isToolAutoAllowed(task, "Write", nil) {
		t.Fatal("Write should NOT be auto-allowed")
	}
}

func TestIsToolAutoAllowed_WebFetch_RespectsDomainAllowlist(t *testing.T) {
	task := tasks.Task{
		SafetyPreset:          "search-browse",
		TaskIntent:            "search-browse",
		ClaudeSandbox:         true,
		ClaudeWebFetchDomains: []string{"docs.claude.com"},
	}

	if !isToolAutoAllowed(task, "WebFetch", json.RawMessage(`{"url":"https://docs.claude.com/en/docs"}`)) {
		t.Fatal("WebFetch should be auto-allowed for allowed domains")
	}
	if isToolAutoAllowed(task, "WebFetch", json.RawMessage(`{"url":"https://example.com"}`)) {
		t.Fatal("WebFetch should NOT be auto-allowed for non-allowlisted domains")
	}
	if isToolAutoAllowed(task, "WebFetch", json.RawMessage(`{"q":"https://example.com"}`)) {
		t.Fatal("WebFetch should NOT be auto-allowed when URL is outside allowlist (q field)")
	}
	if isToolAutoAllowed(task, "WebFetch", json.RawMessage(`{}`)) {
		t.Fatal("WebFetch should NOT be auto-allowed when input URL is missing and allowlist is configured")
	}
}
