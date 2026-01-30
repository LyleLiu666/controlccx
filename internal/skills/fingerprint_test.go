package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirFingerprint_ChangesWithContent_AndIgnoresGitDir(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a"))
	mustWrite(t, filepath.Join(root, "a", "x.txt"), "one\n")

	h1, err := dirFingerprint(root)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	mustWrite(t, filepath.Join(root, "a", "x.txt"), "two\n")
	h2, err := dirFingerprint(root)
	if err != nil {
		t.Fatalf("fingerprint 2: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("expected fingerprint to change")
	}

	mustMkdir(t, filepath.Join(root, ".git"))
	mustWrite(t, filepath.Join(root, ".git", "config"), "ignored\n")
	h3, err := dirFingerprint(root)
	if err != nil {
		t.Fatalf("fingerprint 3: %v", err)
	}
	if h2 != h3 {
		t.Fatalf("expected .git to be ignored")
	}

	// Ensure symlinks do not cause hard failures.
	if err := os.Symlink("missing", filepath.Join(root, "a", "missing-link")); err == nil {
		if _, err := dirFingerprint(root); err != nil {
			t.Fatalf("fingerprint with symlink: %v", err)
		}
	}
}
