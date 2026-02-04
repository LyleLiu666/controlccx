package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
		BaseWorkDir:    repo,
		ConversationID: "2a15e00c-e1ff-4834-974e-61176e720568",
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
		BaseWorkDir:    repo,
		ConversationID: "../../evil",
		WorktreeID:     "w-1",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreate_CopiesIgnoredEnvFilesButSkipsVenv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	repo := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")

	writeFile(t, filepath.Join(repo, "a.txt"), "one\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), ".env\n.venv/\n")
	runGit(t, repo, "add", "a.txt", ".gitignore")
	runGit(t, repo, "commit", "-m", "init")

	writeFile(t, filepath.Join(repo, ".env"), "SECRET=1\n")
	writeFile(t, filepath.Join(repo, ".venv", "pyvenv.cfg"), "home = /usr/bin\n")

	{
		cmd := exec.Command("git", "-C", repo, "check-ignore", "-q", ".env")
		if err := cmd.Run(); err != nil {
			t.Fatalf("expected .env to be ignored: %v", err)
		}
	}
	{
		cmd := exec.Command("git", "-C", repo, "check-ignore", "-q", ".venv/pyvenv.cfg")
		if err := cmd.Run(); err != nil {
			t.Fatalf("expected .venv/pyvenv.cfg to be ignored: %v", err)
		}
	}

	res, err := Create(ctx, CreateOptions{
		BaseWorkDir:    repo,
		ConversationID: "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:     "w-2",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotEnv := readFile(t, filepath.Join(res.Dir, ".env"))
	if gotEnv != "SECRET=1\n" {
		t.Fatalf("worktree .env=%q, want %q", gotEnv, "SECRET=1\n")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, ".venv")); err == nil {
		t.Fatalf("expected worktree .venv not copied")
	}
}

func TestCreate_ExcludesHeavyUntrackedDirsByDefault(t *testing.T) {
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

	writeFile(t, filepath.Join(repo, "b.txt"), "ok\n")
	writeFile(t, filepath.Join(repo, ".venv", "pyvenv.cfg"), "home=/usr/bin\n")
	writeFile(t, filepath.Join(repo, "node_modules", "pkg", "index.js"), "console.log('x')\n")

	res, err := Create(ctx, CreateOptions{
		BaseWorkDir:    repo,
		ConversationID: "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:     "w-3",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotB := readFile(t, filepath.Join(res.Dir, "b.txt")); gotB != "ok\n" {
		t.Fatalf("worktree b.txt=%q, want %q", gotB, "ok\n")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, ".venv")); err == nil {
		t.Fatalf("expected worktree .venv not copied")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "node_modules")); err == nil {
		t.Fatalf("expected worktree node_modules not copied")
	}
}

func TestCreate_UntrackedCapsRequireConfirmationByDefault(t *testing.T) {
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

	// Create a small file but set a tiny cap so we can test force/skip quickly.
	bigPath := filepath.Join(repo, "big.bin")
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatalf("create big: %v", err)
	}
	if err := f.Truncate(128); err != nil {
		_ = f.Close()
		t.Fatalf("truncate: %v", err)
	}
	_ = f.Close()

	_, err = Create(ctx, CreateOptions{
		BaseWorkDir:       repo,
		ConversationID:    "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:        "w-4",
		UntrackedMaxBytes: 64,
	})
	var tooLarge *UntrackedTooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("expected UntrackedTooLargeError, got %v", err)
	}
	if tooLarge.MaxBytes != 64 || tooLarge.Bytes < 128 {
		t.Fatalf("unexpected caps: %+v", tooLarge)
	}

	// Skip untracked and proceed.
	resSkip, err := Create(ctx, CreateOptions{
		BaseWorkDir:       repo,
		ConversationID:    "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:        "w-5",
		Untracked:         UntrackedModeSkip,
		UntrackedMaxBytes: 64,
	})
	if err != nil {
		t.Fatalf("Create skip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(resSkip.Dir, "big.bin")); err == nil {
		t.Fatalf("expected big.bin not copied in skip mode")
	}

	// Force untracked copy and proceed.
	resForce, err := Create(ctx, CreateOptions{
		BaseWorkDir:       repo,
		ConversationID:    "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:        "w-6",
		Untracked:         UntrackedModeForce,
		UntrackedMaxBytes: 64,
	})
	if err != nil {
		t.Fatalf("Create force: %v", err)
	}
	if st, err := os.Stat(filepath.Join(resForce.Dir, "big.bin")); err != nil || st.Size() != 128 {
		t.Fatalf("expected big.bin copied in force mode; err=%v size=%v", err, func() int64 {
			if st == nil {
				return -1
			}
			return st.Size()
		}())
	}
}

func TestCreate_RetriesGitLocks(t *testing.T) {
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

	lockPath := filepath.Join(repo, ".git", "index.lock")
	writeFile(t, lockPath, "")

	done := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = os.Remove(lockPath)
		}
	})
	go func() {
		time.Sleep(250 * time.Millisecond)
		_ = os.Remove(lockPath)
		close(done)
	}()

	_, err := Create(ctx, CreateOptions{
		BaseWorkDir:    repo,
		ConversationID: "2a15e00c-e1ff-4834-974e-61176e720568",
		WorktreeID:     "w-7",
	})
	if err != nil {
		t.Fatalf("expected Create to succeed after lock clears, got: %v", err)
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
