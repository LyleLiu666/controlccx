package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsUnderAnyRoot_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows permissions")
	}

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	link := filepath.Join(root, "out")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	escaped := filepath.Join(link, "x.txt")
	if isUnderAnyRoot(escaped, []FSRoot{{Name: "root", Path: root}}) {
		t.Fatalf("expected symlink escape to be blocked")
	}
}

func TestIsUnderAnyRoot_AllowsNormalChildPath(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if !isUnderAnyRoot(child, []FSRoot{{Name: "root", Path: root}}) {
		t.Fatalf("expected child path to be allowed")
	}
}
