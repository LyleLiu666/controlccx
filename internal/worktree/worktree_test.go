package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreate_CopiesUncommittedChangesIntoWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	repo := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")

	writeFile(t, filepath.Join(repo, "a.txt"), "one\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// Tracked modification + untracked file.
	writeFile(t, filepath.Join(repo, "a.txt"), "two\n")
	writeFile(t, filepath.Join(repo, "b.txt"), "untracked\n")

	res, err := Create(ctx, CreateOptions{
		BaseWorkDir:     repo,
		ConversationID: "c-1",
		WorktreeID:     "w-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if res.Dir == "" {
		t.Fatalf("expected res.Dir set")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, ".git")); err != nil {
		t.Fatalf("expected worktree .git exists: %v", err)
	}

	gotA := readFile(t, filepath.Join(res.Dir, "a.txt"))
	if gotA != "two\n" {
		t.Fatalf("worktree a.txt=%q, want %q", gotA, "two\n")
	}
	gotB := readFile(t, filepath.Join(res.Dir, "b.txt"))
	if gotB != "untracked\n" {
		t.Fatalf("worktree b.txt=%q, want %q", gotB, "untracked\n")
	}

	branch := strings.TrimSpace(runGitOutput(t, res.Dir, "branch", "--show-current"))
	if branch == "" && runtime.GOOS == "windows" {
		// Some older git builds on Windows can behave oddly here; tolerate empty branch name.
		return
	}
	if branch != res.Branch {
		t.Fatalf("worktree branch=%q, want %q", branch, res.Branch)
	}
}

func TestCreate_RejectsConversationIDPathTraversal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	repo := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")

	writeFile(t, filepath.Join(repo, "a.txt"), "one\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	_, err := Create(ctx, CreateOptions{
		BaseWorkDir:     repo,
		ConversationID:  "../../evil",
		WorktreeID:      "w-1",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(ee.Stderr))
		}
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
