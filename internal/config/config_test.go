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
