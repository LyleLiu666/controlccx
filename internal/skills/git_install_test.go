package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestService_GitInstall_MultiSkillsRequiresSelection_AndUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")

	// Two skills under skills/* to trigger MULTI_SKILLS.
	mustMkdir(t, filepath.Join(repo, "skills", "a"))
	mustWrite(t, filepath.Join(repo, "skills", "a", "SKILL.md"), "---\nname: A\ndescription: skill a\n---\n")
	mustMkdir(t, filepath.Join(repo, "skills", "b"))
	mustWrite(t, filepath.Join(repo, "skills", "b", "SKILL.md"), "---\nname: B\ndescription: skill b\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	_, err = svc.InstallGit(ctx, InstallGitInput{RepoURL: repo, Name: "repo-root"})
	if err == nil || !hasPrefix(err.Error(), errPrefixMultiSkills) {
		t.Fatalf("expected %s error, got=%v", errPrefixMultiSkills, err)
	}

	cands, err := svc.ListGitSkills(ctx, repo)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates=%v", cands)
	}

	installed, err := svc.InstallGit(ctx, InstallGitInput{RepoURL: repo, Subpath: "skills/a"})
	if err != nil {
		t.Fatalf("install selection: %v", err)
	}
	if installed.Name != "a" {
		t.Fatalf("installed name=%q", installed.Name)
	}
	if installed.SourceType != sourceTypeGit {
		t.Fatalf("source type=%q", installed.SourceType)
	}

	// Update repo and verify update pulls changes.
	mustWrite(t, filepath.Join(repo, "skills", "a", "SKILL.md"), "---\nname: A\ndescription: changed\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "update a")

	updated, err := svc.UpdateManagedSkill(ctx, "a")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(sourceRoot, "a", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if !strings.Contains(string(b), "changed") {
		t.Fatalf("expected updated content, got=%q", string(b))
	}
	if updated.SourceRevision == "" {
		t.Fatalf("expected source revision")
	}
}

func TestService_GitInstall_RejectsSubpathTraversal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	mustMkdir(t, filepath.Join(repo, "skills", "a"))
	mustWrite(t, filepath.Join(repo, "skills", "a", "SKILL.md"), "---\nname: A\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	_, err = svc.InstallGit(ctx, InstallGitInput{RepoURL: repo, Subpath: "../../etc", Name: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid subpath") {
		t.Fatalf("expected invalid subpath error, got=%v", err)
	}
}

func TestService_GitInstall_AllowsExplicitRootSelection_InMultiSkillRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo-root-skill")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")

	mustWrite(t, filepath.Join(repo, "SKILL.md"), "---\nname: RootSkill\ndescription: root\n---\n")
	mustMkdir(t, filepath.Join(repo, "skills", "a"))
	mustWrite(t, filepath.Join(repo, "skills", "a", "SKILL.md"), "---\nname: A\ndescription: skill a\n---\n")
	mustMkdir(t, filepath.Join(repo, "skills", "b"))
	mustWrite(t, filepath.Join(repo, "skills", "b", "SKILL.md"), "---\nname: B\ndescription: skill b\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	installed, err := svc.InstallGit(ctx, InstallGitInput{RepoURL: repo, Subpath: ".", Name: "root"})
	if err != nil {
		t.Fatalf("install root selection: %v", err)
	}
	if installed.Name != "root" {
		t.Fatalf("name=%q", installed.Name)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "root", "SKILL.md")); err != nil {
		t.Fatalf("expected root SKILL.md: %v", err)
	}
}

func TestService_GitInstall_MultiSkillsRequiresSelection_PluginsLayout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo-plugins")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")

	mustMkdir(t, filepath.Join(repo, "plugins", "p1", "skills", "a"))
	mustWrite(t, filepath.Join(repo, "plugins", "p1", "skills", "a", "SKILL.md"), "---\nname: A\ndescription: plugin a\n---\n")
	mustMkdir(t, filepath.Join(repo, "plugins", "p2", "skills", "b"))
	mustWrite(t, filepath.Join(repo, "plugins", "p2", "skills", "b", "SKILL.md"), "---\nname: B\ndescription: plugin b\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	_, err = svc.InstallGit(ctx, InstallGitInput{RepoURL: repo, Name: "repo-root"})
	if err == nil || !hasPrefix(err.Error(), errPrefixMultiSkills) {
		t.Fatalf("expected %s error, got=%v", errPrefixMultiSkills, err)
	}

	cands, err := svc.ListGitSkills(ctx, repo)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates=%v", cands)
	}

	seen := make(map[string]bool, len(cands))
	for _, c := range cands {
		seen[c.Subpath] = true
	}
	if !seen["plugins/p1/skills/a"] || !seen["plugins/p2/skills/b"] {
		t.Fatalf("unexpected subpaths=%v", cands)
	}
}

func TestService_GitInstall_AutoSelectsSingleSkill_FromPluginsLayout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo-single-plugin")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")

	mustMkdir(t, filepath.Join(repo, "plugins", "p1", "skills", "only-skill"))
	mustWrite(t, filepath.Join(repo, "plugins", "p1", "skills", "only-skill", "SKILL.md"), "---\nname: Only\ndescription: single\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	installed, err := svc.InstallGit(ctx, InstallGitInput{RepoURL: repo})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.Name != "only-skill" {
		t.Fatalf("name=%q", installed.Name)
	}
	if installed.SourceSubpath != "plugins/p1/skills/only-skill" {
		t.Fatalf("subpath=%q", installed.SourceSubpath)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "only-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected installed SKILL.md: %v", err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
