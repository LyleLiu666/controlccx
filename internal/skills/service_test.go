package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestService_ListAndLinkUnlink(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "skill-creator"))
	mustWrite(t, filepath.Join(sourceRoot, "skill-creator", "README.md"), "hello\n")

	svc, err := NewService(Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
		CodexHome:   filepath.Join(home, ".codex2"),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	before, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(before.Skills) != 1 || before.Skills[0].Name != "skill-creator" {
		t.Fatalf("skills=%v", before.Skills)
	}

	// Link to Claude target.
	if err := svc.Link(ctx, "skill-creator", TargetClaudeCode); err != nil {
		t.Fatalf("link claude: %v", err)
	}
	claudePath := filepath.Join(home, ".claude", "skills", "skill-creator")
	assertSymlink(t, claudePath)

	// Link to Codex should affect both roots.
	if err := svc.Link(ctx, "skill-creator", TargetCodex); err != nil {
		t.Fatalf("link codex: %v", err)
	}
	assertSymlink(t, filepath.Join(home, ".codex", "skills", "skill-creator"))
	assertSymlink(t, filepath.Join(home, ".codex2", "skills", "skill-creator"))

	after, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	got := after.Skills[0]
	if got.Name != "skill-creator" {
		t.Fatalf("name=%q", got.Name)
	}
	if len(got.Targets) == 0 {
		t.Fatalf("expected target statuses")
	}

	// Unlink should remove targets but keep source intact.
	if err := svc.Unlink(ctx, "skill-creator", TargetClaudeCode); err != nil {
		t.Fatalf("unlink claude: %v", err)
	}
	if _, err := os.Lstat(claudePath); !os.IsNotExist(err) {
		t.Fatalf("expected claude link removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "skill-creator")); err != nil {
		t.Fatalf("expected source still exists: %v", err)
	}
}

func TestService_BrokenLinkDetected(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdir(t, sourceRoot)

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	targetRoot := filepath.Join(home, ".claude", "skills")
	mustMkdir(t, targetRoot)
	broken := filepath.Join(targetRoot, "x")
	if err := os.Symlink("missing-target", broken); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	out, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Skills) != 1 || out.Skills[0].Name != "x" {
		t.Fatalf("skills=%v", out.Skills)
	}
	foundBroken := false
	for _, st := range out.Skills[0].Targets {
		if st.Target == TargetClaudeCode && st.Status == StatusBroken {
			foundBroken = true
		}
	}
	if !foundBroken {
		t.Fatalf("expected broken status in %v", out.Skills[0].Targets)
	}
}

func TestService_CopyFallback(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "skill-creator"))
	mustWrite(t, filepath.Join(sourceRoot, "skill-creator", "README.md"), "hello\n")

	svc, err := NewService(Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
		Symlink: func(_, _ string) error {
			return errors.New("no symlink")
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.Link(ctx, "skill-creator", TargetClaudeCode); err != nil {
		t.Fatalf("link: %v", err)
	}
	destDir := filepath.Join(home, ".claude", "skills", "skill-creator")
	info, err := os.Stat(destDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected copied dir, err=%v info=%v", err, info)
	}
	if _, err := os.Stat(filepath.Join(destDir, managedMarkerFile)); err != nil {
		t.Fatalf("expected marker file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "README.md")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}

	if err := svc.Unlink(ctx, "skill-creator", TargetClaudeCode); err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Fatalf("expected removed, err=%v", err)
	}
}

func TestService_SyncCursor_ForcesCopy_AndSupportsOverwrite(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "demo"))
	mustWrite(t, filepath.Join(sourceRoot, "demo", "README.md"), "hello\n")

	cursorRoot := filepath.Join(home, ".cursor", "skills")
	mustMkdir(t, filepath.Join(cursorRoot, "demo"))
	mustWrite(t, filepath.Join(cursorRoot, "demo", "unmanaged.txt"), "x\n")

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.Sync(ctx, "demo", TargetCursor, false)
	if err == nil || !hasPrefix(err.Error(), errPrefixTargetExists) {
		t.Fatalf("expected %s error, got=%v", errPrefixTargetExists, err)
	}

	if err := svc.Sync(ctx, "demo", TargetCursor, true); err != nil {
		t.Fatalf("sync overwrite: %v", err)
	}

	dest := filepath.Join(cursorRoot, "demo")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("lstat dest: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected copy (not symlink) for cursor, mode=%v", fi.Mode())
	}
	if !fi.IsDir() {
		t.Fatalf("expected dest dir, mode=%v", fi.Mode())
	}
	if _, err := os.Stat(filepath.Join(dest, managedMarkerFile)); err != nil {
		t.Fatalf("expected marker file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertSymlink(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s, mode=%v", path, fi.Mode())
	}
}
