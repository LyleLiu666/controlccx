package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_ParsesFSRoots(t *testing.T) {
	dataDir := t.TempDir()
	cfgPath := filepath.Join(dataDir, "config.yaml")
	content := `
server:
  addr: 127.0.0.1:5174
fs_roots:
  - /tmp/a
  - /tmp/b
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dataDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	want := []string{"/tmp/a", "/tmp/b"}
	if !reflect.DeepEqual(cfg.FSRoots, want) {
		t.Fatalf("fs_roots=%v want %v", cfg.FSRoots, want)
	}
}

func TestDefault_SecretaryFlags(t *testing.T) {
	cfg := Default()
	if !cfg.Secretary.ConversationMemoryEnabled {
		t.Fatalf("conversation_memory_enabled default=false want true")
	}
	if !cfg.Secretary.WriteGuardEnabled {
		t.Fatalf("write_guard_enabled default=false want true")
	}
	if cfg.Secretary.ProactiveEnabled != "conservative" {
		t.Fatalf("proactive_enabled=%q want %q", cfg.Secretary.ProactiveEnabled, "conservative")
	}
}

func TestLoad_ParsesSecretaryFlags(t *testing.T) {
	dataDir := t.TempDir()
	cfgPath := filepath.Join(dataDir, "config.yaml")
	content := `
secretary:
  conversation_memory_enabled: false
  write_guard_enabled: false
  proactive_enabled: aggressive
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(dataDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Secretary.ConversationMemoryEnabled {
		t.Fatalf("conversation_memory_enabled=true want false")
	}
	if cfg.Secretary.WriteGuardEnabled {
		t.Fatalf("write_guard_enabled=true want false")
	}
	if cfg.Secretary.ProactiveEnabled != "aggressive" {
		t.Fatalf("proactive_enabled=%q want %q", cfg.Secretary.ProactiveEnabled, "aggressive")
	}
}
