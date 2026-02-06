package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportCodexLive_ReadsAuthAndConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-test-abcdef","tokens":{}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"gpt-5.3-codex\"\nmodel_reasoning_effort = \"xhigh\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	out, err := ImportCodexLive(dir)
	if err != nil {
		t.Fatalf("ImportCodexLive: %v", err)
	}
	if out.AuthPath == "" || out.ConfigPath == "" {
		t.Fatalf("expected paths to be set: %+v", out)
	}
	if out.Target.APIKey != "sk-test-abcdef" {
		t.Fatalf("api key got=%q", out.Target.APIKey)
	}
	if out.Target.Model != "gpt-5.3-codex" {
		t.Fatalf("model got=%q", out.Target.Model)
	}
	if out.Target.ReasoningEffort != "xhigh" {
		t.Fatalf("effort got=%q", out.Target.ReasoningEffort)
	}
}

func TestImportCodexLive_ConfigOptional(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-test-abcdef"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	out, err := ImportCodexLive(dir)
	if err != nil {
		t.Fatalf("ImportCodexLive: %v", err)
	}
	if out.AuthPath == "" {
		t.Fatalf("expected auth path")
	}
	if out.ConfigPath != "" {
		t.Fatalf("expected empty config path")
	}
	if out.Target.APIKey == "" {
		t.Fatalf("expected api key")
	}
}

func TestImportCodexLive_InvalidAuthJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if _, err := ImportCodexLive(dir); err == nil {
		t.Fatalf("expected error")
	}
}
