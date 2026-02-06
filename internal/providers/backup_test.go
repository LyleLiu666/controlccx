package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateRotatingBackup_RotatesAndPreservesPerms(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "providers.json")
	backupDir := filepath.Join(dir, "backups", "providers")

	if err := os.WriteFile(src, []byte("{\"profiles\":[]}\n"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	b1, err := CreateRotatingBackup(src, backupDir, 2)
	if err != nil {
		t.Fatalf("backup1: %v", err)
	}
	if b1 == "" {
		t.Fatalf("expected backup path")
	}
	if err := os.WriteFile(src, []byte("{\"profiles\":[{\"id\":\"p1\"}]}\n"), 0o600); err != nil {
		t.Fatalf("write src2: %v", err)
	}

	b2, err := CreateRotatingBackup(src, backupDir, 2)
	if err != nil {
		t.Fatalf("backup2: %v", err)
	}
	if b2 == "" {
		t.Fatalf("expected backup path")
	}

	if err := os.WriteFile(src, []byte("{\"profiles\":[{\"id\":\"p2\"}]}\n"), 0o600); err != nil {
		t.Fatalf("write src3: %v", err)
	}
	b3, err := CreateRotatingBackup(src, backupDir, 2)
	if err != nil {
		t.Fatalf("backup3: %v", err)
	}
	if b3 == "" {
		t.Fatalf("expected backup path")
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if got := len(entries); got != 2 {
		t.Fatalf("expected 2 backups kept, got %d", got)
	}

	if _, err := os.Stat(b1); err == nil {
		t.Fatalf("expected oldest backup to be removed")
	}
	if _, err := os.Stat(b3); err != nil {
		t.Fatalf("expected newest backup to exist: %v", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(b3)
		if err != nil {
			t.Fatalf("stat newest: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("newest perm got=%#o want=%#o", got, 0o600)
		}
	}
}

func TestCreateRotatingBackup_SrcMissingIsNoop(t *testing.T) {
	dir := t.TempDir()
	got, err := CreateRotatingBackup(filepath.Join(dir, "missing.json"), filepath.Join(dir, "b"), 10)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty backup path, got %q", got)
	}
}
