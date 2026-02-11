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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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

func TestService_SyncOverwrite_BacksUpUnmanagedEntry(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "demo"))
	mustWrite(t, filepath.Join(sourceRoot, "demo", "README.md"), "hello\n")

	claudeRoot := filepath.Join(home, ".claude", "skills")
	mustMkdir(t, filepath.Join(claudeRoot, "demo"))
	mustWrite(t, filepath.Join(claudeRoot, "demo", "unmanaged.txt"), "x\n")

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.Sync(ctx, "demo", TargetClaudeCode, true); err != nil {
		t.Fatalf("sync overwrite: %v", err)
	}

	dest := filepath.Join(claudeRoot, "demo")
	assertSymlink(t, dest)

	// Original unmanaged entry should be backed up.
	backupRoot := filepath.Join(home, ".controlccx", "skills_backups", string(TargetClaudeCode), "demo")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("read backup root: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected backup entry in %s", backupRoot)
	}
	backupPath := filepath.Join(backupRoot, entries[0].Name())
	b, err := os.ReadFile(filepath.Join(backupPath, "unmanaged.txt"))
	if err != nil {
		t.Fatalf("read backed up file: %v", err)
	}
	if string(b) != "x\n" {
		t.Fatalf("backup unmanaged.txt=%q", string(b))
	}

	if _, err := os.Stat(filepath.Join(dest, "unmanaged.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected unmanaged.txt removed from destination, err=%v", err)
	}
}

func TestService_ListDerivesRepoMetadataAndFacetForGitSource(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	gitA := filepath.Join(sourceRoot, "git-a")
	gitB := filepath.Join(sourceRoot, "git-b")
	gitC := filepath.Join(sourceRoot, "git-c")
	gitD := filepath.Join(sourceRoot, "git-d")
	localA := filepath.Join(sourceRoot, "local-a")
	mustMkdir(t, gitA)
	mustMkdir(t, gitB)
	mustMkdir(t, gitC)
	mustMkdir(t, gitD)
	mustMkdir(t, localA)

	if err := writeManagedManifest(gitA, ManagedSkillManifest{
		Name:       "git-a",
		SourceType: sourceTypeGit,
		SourceRef:  "https://github.com/Acme/Repo.git",
	}); err != nil {
		t.Fatalf("manifest git-a: %v", err)
	}
	if err := writeManagedManifest(gitB, ManagedSkillManifest{
		Name:       "git-b",
		SourceType: sourceTypeGit,
		SourceRef:  "acme/repo",
	}); err != nil {
		t.Fatalf("manifest git-b: %v", err)
	}
	if err := writeManagedManifest(gitC, ManagedSkillManifest{
		Name:       "git-c",
		SourceType: sourceTypeGit,
		SourceRef:  "git@gitlab.com:Team/Repo.git",
	}); err != nil {
		t.Fatalf("manifest git-c: %v", err)
	}
	if err := writeManagedManifest(gitD, ManagedSkillManifest{
		Name:       "git-d",
		SourceType: sourceTypeGit,
		SourceRef:  "https://gitlab.com/team/repo.git",
	}); err != nil {
		t.Fatalf("manifest git-d: %v", err)
	}
	if err := writeManagedManifest(localA, ManagedSkillManifest{
		Name:       "local-a",
		SourceType: sourceTypeLocal,
		SourceRef:  filepath.Join(home, "src-local-a"),
	}); err != nil {
		t.Fatalf("manifest local-a: %v", err)
	}

	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	out, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byName := make(map[string]Skill, len(out.Skills))
	for _, s := range out.Skills {
		byName[s.Name] = s
	}

	a := byName["git-a"]
	b := byName["git-b"]
	cg := byName["git-c"]
	dg := byName["git-d"]
	c := byName["local-a"]
	if a.RepoKey == "" || a.RepoLabel == "" || a.RepoRef == "" {
		t.Fatalf("git-a repo metadata missing: %+v", a)
	}
	if b.RepoKey == "" || b.RepoLabel == "" || b.RepoRef == "" {
		t.Fatalf("git-b repo metadata missing: %+v", b)
	}
	if a.RepoKey != b.RepoKey {
		t.Fatalf("expected same repo key for git-a/git-b, got %q vs %q", a.RepoKey, b.RepoKey)
	}
	if cg.RepoKey == "" || dg.RepoKey == "" {
		t.Fatalf("expected git-c/git-d repo metadata, got %+v %+v", cg, dg)
	}
	if cg.RepoKey != dg.RepoKey {
		t.Fatalf("expected same repo key for git-c/git-d, got %q vs %q", cg.RepoKey, dg.RepoKey)
	}
	if c.RepoKey != "" || c.RepoLabel != "" || c.RepoRef != "" {
		t.Fatalf("expected local-a no repo metadata, got %+v", c)
	}

	if len(out.Repos) != 2 {
		t.Fatalf("repos=%d, want 2 (%+v)", len(out.Repos), out.Repos)
	}
	found := map[string]int{}
	for _, r := range out.Repos {
		found[r.Key] = r.Count
	}
	if found[a.RepoKey] != 2 {
		t.Fatalf("facet count for %q=%d, want 2", a.RepoKey, found[a.RepoKey])
	}
	if found[cg.RepoKey] != 2 {
		t.Fatalf("facet count for %q=%d, want 2", cg.RepoKey, found[cg.RepoKey])
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
