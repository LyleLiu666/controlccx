package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportClaudeLive_ExtractsEnvFields(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{
  "env": {
    "ANTHROPIC_BASE_URL": " https://example.com/api/anthropic ",
    "ANTHROPIC_AUTH_TOKEN": " sk-ant-test ",
    "ANTHROPIC_MODEL": " claude-3-7-sonnet ",
    "ANTHROPIC_SMALL_FAST_MODEL": " claude-3-5-haiku ",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
`), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	out, err := ImportClaudeLive(dir)
	if err != nil {
		t.Fatalf("ImportClaudeLive: %v", err)
	}
	if out.SettingsPath != settingsPath {
		t.Fatalf("settings path got=%q want=%q", out.SettingsPath, settingsPath)
	}
	if out.Target.BaseURL != "https://example.com/api/anthropic" {
		t.Fatalf("base url got=%q", out.Target.BaseURL)
	}
	if out.Target.AuthToken != "sk-ant-test" {
		t.Fatalf("auth token got=%q", out.Target.AuthToken)
	}
	if out.Target.Model != "claude-3-7-sonnet" {
		t.Fatalf("model got=%q", out.Target.Model)
	}
	if out.Target.SmallFastModel != "claude-3-5-haiku" {
		t.Fatalf("small fast model got=%q", out.Target.SmallFastModel)
	}
}

func TestImportClaudeLive_MissingFileIsOk(t *testing.T) {
	out, err := ImportClaudeLive(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.SettingsPath != "" {
		t.Fatalf("expected empty settings path, got %q", out.SettingsPath)
	}
}
