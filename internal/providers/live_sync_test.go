package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCodexLive_WritesAuthAndConfigAndBackups(t *testing.T) {
	dir := t.TempDir()
	codexHome := filepath.Join(dir, "codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-old","tokens":{"a":1}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("# header\nmodel = \"old\"\n[foo]\nbar = \"baz\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	backupRoot := filepath.Join(dir, "backups")
	if err := SyncCodexLive(codexHome, CodexTarget{
		APIKey:          "sk-test-abcdef",
		Model:           "gpt-5.3-codex",
		ReasoningEffort: "xhigh",
	}, LiveSyncOptions{BackupDir: backupRoot, Keep: 10}); err != nil {
		t.Fatalf("SyncCodexLive: %v", err)
	}

	{
		b, err := os.ReadFile(filepath.Join(codexHome, "auth.json"))
		if err != nil {
			t.Fatalf("read auth.json: %v", err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("parse auth.json: %v", err)
		}
		if got, _ := v["OPENAI_API_KEY"].(string); got != "sk-test-abcdef" {
			t.Fatalf("api key got=%q", got)
		}
		if _, ok := v["tokens"]; !ok {
			t.Fatalf("expected tokens to be preserved")
		}
	}
	{
		b, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
		if err != nil {
			t.Fatalf("read config.toml: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, "model = \"gpt-5.3-codex\"") {
			t.Fatalf("expected model to be updated, got:\n%s", s)
		}
		if !strings.Contains(s, "model_reasoning_effort = \"xhigh\"") {
			t.Fatalf("expected effort to be set, got:\n%s", s)
		}
		if !strings.Contains(s, "[foo]\nbar = \"baz\"") {
			t.Fatalf("expected section to be preserved, got:\n%s", s)
		}
	}

	{
		entries, err := os.ReadDir(filepath.Join(backupRoot, "codex", "auth"))
		if err != nil {
			t.Fatalf("readdir auth backups: %v", err)
		}
		if len(entries) == 0 {
			t.Fatalf("expected auth backups")
		}
	}
	{
		entries, err := os.ReadDir(filepath.Join(backupRoot, "codex", "config"))
		if err != nil {
			t.Fatalf("readdir config backups: %v", err)
		}
		if len(entries) == 0 {
			t.Fatalf("expected config backups")
		}
	}
}

func TestSyncCodexLive_InvalidAuthRefusesUnlessForce(t *testing.T) {
	dir := t.TempDir()
	codexHome := filepath.Join(dir, "codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	backupRoot := filepath.Join(dir, "backups")
	if err := SyncCodexLive(codexHome, CodexTarget{APIKey: "sk-test"}, LiveSyncOptions{BackupDir: backupRoot}); err == nil {
		t.Fatalf("expected error")
	}
	{
		b, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatalf("read auth.json: %v", err)
		}
		if string(b) != "{" {
			t.Fatalf("expected auth.json unchanged")
		}
	}
	if _, err := os.Stat(filepath.Join(backupRoot, "codex")); err == nil {
		t.Fatalf("expected no backups on refusal")
	}

	if err := SyncCodexLive(codexHome, CodexTarget{APIKey: "sk-test"}, LiveSyncOptions{BackupDir: backupRoot, Force: true}); err != nil {
		t.Fatalf("SyncCodexLive(force): %v", err)
	}
	{
		b, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatalf("read auth.json after force: %v", err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("parse auth.json after force: %v", err)
		}
		if got, _ := v["OPENAI_API_KEY"].(string); got != "sk-test" {
			t.Fatalf("api key got=%q", got)
		}
	}
	{
		entries, err := os.ReadDir(filepath.Join(backupRoot, "codex", "auth"))
		if err != nil {
			t.Fatalf("readdir auth backups: %v", err)
		}
		if len(entries) == 0 {
			t.Fatalf("expected auth backups after force")
		}
	}
}

func TestSyncCodexLive_SkipsConfigWhenNoModelOrEffort(t *testing.T) {
	dir := t.TempDir()
	codexHome := filepath.Join(dir, "codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-old"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := SyncCodexLive(codexHome, CodexTarget{APIKey: "sk-new"}, LiveSyncOptions{}); err != nil {
		t.Fatalf("SyncCodexLive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); err == nil {
		t.Fatalf("expected config.toml to not be created")
	}
}

func TestSyncClaudeLive_PrefersExistingClaudeJSON(t *testing.T) {
	dir := t.TempDir()
	claudeHome := filepath.Join(dir, "claude")
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatalf("mkdir claude home: %v", err)
	}
	claudeJSONPath := filepath.Join(claudeHome, "claude.json")
	if err := os.WriteFile(claudeJSONPath, []byte(`{"theme":"dark","env":{"ANTHROPIC_API_KEY":"sk-old","OTHER":"x"}}`), 0o600); err != nil {
		t.Fatalf("write claude.json: %v", err)
	}

	backupRoot := filepath.Join(dir, "backups")
	if err := SyncClaudeLive(claudeHome, ClaudeTarget{
		BaseURL:   "https://example.com/api/anthropic",
		AuthToken: "sk-ant-test",
		Model:     "claude-3-7-sonnet",
	}, LiveSyncOptions{BackupDir: backupRoot}); err != nil {
		t.Fatalf("SyncClaudeLive: %v", err)
	}

	if _, err := os.Stat(filepath.Join(claudeHome, "settings.json")); err == nil {
		t.Fatalf("expected settings.json to not be created when claude.json exists")
	}

	{
		b, err := os.ReadFile(claudeJSONPath)
		if err != nil {
			t.Fatalf("read claude.json: %v", err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("parse claude.json: %v", err)
		}
		env, _ := v["env"].(map[string]any)
		if env == nil {
			t.Fatalf("expected env")
		}
		if got, _ := env["ANTHROPIC_API_KEY"].(string); got != "sk-old" {
			t.Fatalf("expected existing api key preserved, got=%q", got)
		}
		if got, _ := env["ANTHROPIC_BASE_URL"].(string); got != "https://example.com/api/anthropic" {
			t.Fatalf("base url got=%q", got)
		}
		if got, _ := env["ANTHROPIC_AUTH_TOKEN"].(string); got != "sk-ant-test" {
			t.Fatalf("auth token got=%q", got)
		}
		if got, _ := env["ANTHROPIC_MODEL"].(string); got != "claude-3-7-sonnet" {
			t.Fatalf("model got=%q", got)
		}
		if got, _ := v["theme"].(string); got != "dark" {
			t.Fatalf("expected unrelated fields preserved")
		}
	}
	{
		entries, err := os.ReadDir(filepath.Join(backupRoot, "claude"))
		if err != nil {
			t.Fatalf("readdir claude backups: %v", err)
		}
		if len(entries) == 0 {
			t.Fatalf("expected claude backups")
		}
	}
}

func TestSyncClaudeLive_InvalidSettingsRefusesUnlessForce(t *testing.T) {
	dir := t.TempDir()
	claudeHome := filepath.Join(dir, "claude")
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatalf("mkdir claude home: %v", err)
	}
	settingsPath := filepath.Join(claudeHome, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	backupRoot := filepath.Join(dir, "backups")
	if err := SyncClaudeLive(claudeHome, ClaudeTarget{AuthToken: "sk-ant-test"}, LiveSyncOptions{BackupDir: backupRoot}); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := os.Stat(filepath.Join(backupRoot, "claude")); err == nil {
		t.Fatalf("expected no backups on refusal")
	}

	if err := SyncClaudeLive(claudeHome, ClaudeTarget{AuthToken: "sk-ant-test"}, LiveSyncOptions{BackupDir: backupRoot, Force: true}); err != nil {
		t.Fatalf("SyncClaudeLive(force): %v", err)
	}
	{
		b, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read settings.json after force: %v", err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("parse settings.json after force: %v", err)
		}
		env, _ := v["env"].(map[string]any)
		if env == nil {
			t.Fatalf("expected env")
		}
		if got, _ := env["ANTHROPIC_AUTH_TOKEN"].(string); got != "sk-ant-test" {
			t.Fatalf("auth token got=%q", got)
		}
	}
}
