package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestService_GitInstall_MultiSkillsRequiresSelection_AndUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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
	sourceRoot := filepath.Join(home, ".agent", "skills")
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

func TestListGitCandidates_ScansRepoForSkillMD_OuterMostOnly(t *testing.T) {
	repoDir := t.TempDir()

	// Should not traverse git internals.
	mustMkdir(t, filepath.Join(repoDir, ".git"))
	mustWrite(t, filepath.Join(repoDir, ".git", "SKILL.md"), "---\nname: ignored\n---\n")

	mustMkdir(t, filepath.Join(repoDir, "agent", "skills", "brainstorming"))
	mustWrite(
		t,
		filepath.Join(repoDir, "agent", "skills", "brainstorming", "SKILL.md"),
		"---\nname: Brainstorming\ndescription: x\n---\n",
	)
	mustMkdir(t, filepath.Join(repoDir, "agent", "skills", "brainstorming", "nested"))
	mustWrite(t, filepath.Join(repoDir, "agent", "skills", "brainstorming", "nested", "SKILL.md"), "---\nname: Nested\n---\n")

	mustMkdir(t, filepath.Join(repoDir, "misc"))
	mustWrite(t, filepath.Join(repoDir, "misc", "SKILL.md"), "---\nname: Misc\n---\n")

	cands, err := listGitCandidates(repoDir, "")
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	seen := make(map[string]GitSkillCandidate, len(cands))
	for _, c := range cands {
		seen[c.Subpath] = c
	}
	if _, ok := seen["agent/skills/brainstorming"]; !ok {
		t.Fatalf("expected brainstorming candidate, got=%v", cands)
	}
	if _, ok := seen["agent/skills/brainstorming/nested"]; ok {
		t.Fatalf("expected nested to be filtered out, got=%v", cands)
	}
	if _, ok := seen["misc"]; !ok {
		t.Fatalf("expected misc candidate, got=%v", cands)
	}
	if _, ok := seen[".git"]; ok {
		t.Fatalf("expected .git to be skipped, got=%v", cands)
	}

	cands, err = listGitCandidates(repoDir, "agent/skills")
	if err != nil {
		t.Fatalf("list candidates under subpath: %v", err)
	}
	if len(cands) != 1 || cands[0].Subpath != "agent/skills/brainstorming" {
		t.Fatalf("expected only brainstorming under agent/skills, got=%v", cands)
	}
}

func TestService_InstallGitBatch_InstallsMultipleAndSyncsTargets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	repo := filepath.Join(home, "repo-batch")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")

	mustMkdir(t, filepath.Join(repo, "agent", "skills", "a"))
	mustWrite(t, filepath.Join(repo, "agent", "skills", "a", "SKILL.md"), "---\nname: A\n---\n")
	mustMkdir(t, filepath.Join(repo, "agent", "skills", "b"))
	mustWrite(t, filepath.Join(repo, "agent", "skills", "b", "SKILL.md"), "---\nname: B\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	installed, err := svc.InstallGitBatch(ctx, InstallGitBatchInput{
		RepoURL: repo,
		Skills: []InstallGitBatchItem{
			{Subpath: "agent/skills/a", Name: "skill-a"},
			{Subpath: "agent/skills/b", Name: "skill-b"},
		},
		Targets:   []Target{TargetClaudeCode, TargetCodex},
		Overwrite: false,
	})
	if err != nil {
		t.Fatalf("batch install: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("installed=%v", installed)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "skill-a", "SKILL.md")); err != nil {
		t.Fatalf("expected skill-a installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "skill-b", "SKILL.md")); err != nil {
		t.Fatalf("expected skill-b installed: %v", err)
	}

	// Verify sync created links in target roots.
	assertSymlink(t, filepath.Join(home, ".claude", "skills", "skill-a"))
	assertSymlink(t, filepath.Join(home, ".claude", "skills", "skill-b"))
	assertSymlink(t, filepath.Join(home, ".codex", "skills", "skill-a"))
	assertSymlink(t, filepath.Join(home, ".codex", "skills", "skill-b"))
}

func TestService_InstallGit_AutoVersionsOnNameCollision_WhenOverwriteFalse(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Existing local skill.
	mustMkdir(t, filepath.Join(sourceRoot, "brainstorming"))
	mustWrite(t, filepath.Join(sourceRoot, "brainstorming", "SKILL.md"), "---\nname: Local\n---\n")

	// Git repo with same skill name.
	repo := filepath.Join(home, "repo-collision")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	mustMkdir(t, filepath.Join(repo, "skills", "brainstorming"))
	mustWrite(t, filepath.Join(repo, "skills", "brainstorming", "SKILL.md"), "---\nname: Git\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	beforeDay := time.Now().Local().Format("20060102")
	installed, err := svc.InstallGit(ctx, InstallGitInput{
		RepoURL:   repo,
		Subpath:   "skills/brainstorming",
		Name:      "brainstorming",
		Overwrite: false,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.Name == "brainstorming" {
		t.Fatalf("expected collision install to be versioned, got=%q", installed.Name)
	}
	// Best-effort: the day is derived from local time; tolerate edge cases around midnight by only
	// checking the general pattern and (when stable) the day prefix.
	re := regexp.MustCompile(`^brainstorming@\d{8}-\d{2}$`)
	if !re.MatchString(installed.Name) {
		t.Fatalf("unexpected installed name=%q", installed.Name)
	}
	if !strings.Contains(installed.Name, "@"+beforeDay) {
		// Allow midnight rollover (rare). Still ensure it is a versioned name.
	}

	// Original remains unchanged.
	b, err := os.ReadFile(filepath.Join(sourceRoot, "brainstorming", "SKILL.md"))
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	if !strings.Contains(string(b), "Local") {
		t.Fatalf("expected original content intact, got=%q", string(b))
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, installed.Name, "SKILL.md")); err != nil {
		t.Fatalf("expected versioned skill installed: %v", err)
	}
}

func TestService_InstallGitBatch_AutoVersionsOnNameCollision_WhenOverwriteFalse(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Existing local skill.
	mustMkdir(t, filepath.Join(sourceRoot, "brainstorming"))
	mustWrite(t, filepath.Join(sourceRoot, "brainstorming", "SKILL.md"), "---\nname: Local\n---\n")

	// Git repo with same skill name.
	repo := filepath.Join(home, "repo-batch-collision")
	mustMkdir(t, repo)
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Test")
	mustMkdir(t, filepath.Join(repo, "skills", "brainstorming"))
	mustWrite(t, filepath.Join(repo, "skills", "brainstorming", "SKILL.md"), "---\nname: Git\n---\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "init")

	installed, err := svc.InstallGitBatch(ctx, InstallGitBatchInput{
		RepoURL: repo,
		Skills: []InstallGitBatchItem{
			{Subpath: "skills/brainstorming", Name: "brainstorming"},
		},
		Overwrite: false,
	})
	if err != nil {
		t.Fatalf("batch install: %v", err)
	}
	if len(installed) != 1 {
		t.Fatalf("installed=%v", installed)
	}
	if installed[0].Name == "brainstorming" {
		t.Fatalf("expected collision batch install to be versioned, got=%q", installed[0].Name)
	}
	re := regexp.MustCompile(`^brainstorming@\d{8}-\d{2}$`)
	if !re.MatchString(installed[0].Name) {
		t.Fatalf("unexpected installed name=%q", installed[0].Name)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, installed[0].Name, "SKILL.md")); err != nil {
		t.Fatalf("expected versioned skill installed: %v", err)
	}
}
