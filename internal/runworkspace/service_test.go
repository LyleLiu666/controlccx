package runworkspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"

	"github.com/google/uuid"
)

func TestService_EnsureForTask_CopyModeAndApplyBackConflicts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	svc := NewService(store, Options{Retain: 5})
	task := tasks.Task{
		ID:             "t-1",
		ConversationID: uuid.NewString(),
		WorkDir:        base,
	}
	ens, err := svc.EnsureForTask(ctx, task)
	if err != nil {
		t.Fatalf("EnsureForTask: %v", err)
	}
	if ens.Workspace.Kind != "copy" {
		t.Fatalf("kind=%q, want %q", ens.Workspace.Kind, "copy")
	}
	if !strings.Contains(ens.Workspace.RunWorkDir, filepath.Join(base, ".ccx", "workspaces")) {
		t.Fatalf("run_workdir=%q, want under .ccx/workspaces", ens.Workspace.RunWorkDir)
	}
	if _, err := os.Stat(filepath.Join(ens.Workspace.RunRoot, "manifest.json")); err != nil {
		t.Fatalf("expected manifest.json: %v", err)
	}

	// Modify both base and workspace versions of the same file -> conflict.
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ens.Workspace.RunRoot, "a.txt"), []byte("ws\n"), 0o644); err != nil {
		t.Fatalf("write ws a.txt: %v", err)
	}

	res, err := svc.Merge(ctx, ens.Workspace.Key)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0] != "a.txt" {
		t.Fatalf("conflicts=%v, want [a.txt]", res.Conflicts)
	}
	got, err := os.ReadFile(filepath.Join(base, "a.txt"))
	if err != nil {
		t.Fatalf("read base a.txt: %v", err)
	}
	if string(got) != "base\n" {
		t.Fatalf("base a.txt=%q, want %q", string(got), "base\\n")
	}
}

func TestService_EnsureForTask_GitWorktreeModeCopiesUncommitted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// Modify tracked + add untracked.
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write a2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	svc := NewService(store, Options{Retain: 5})
	task := tasks.Task{
		ID:             "t-2",
		ConversationID: uuid.NewString(),
		WorkDir:        repo,
	}
	ens, err := svc.EnsureForTask(ctx, task)
	if err != nil {
		t.Fatalf("EnsureForTask: %v", err)
	}
	if ens.Workspace.Kind != "git-worktree" {
		t.Fatalf("kind=%q, want %q", ens.Workspace.Kind, "git-worktree")
	}
	if _, err := os.Stat(filepath.Join(ens.Workspace.RunRoot, ".git")); err != nil {
		t.Fatalf("expected worktree .git exists: %v", err)
	}
	gotA, err := os.ReadFile(filepath.Join(ens.Workspace.RunWorkDir, "a.txt"))
	if err != nil {
		t.Fatalf("read ws a.txt: %v", err)
	}
	if string(gotA) != "two\n" {
		t.Fatalf("ws a.txt=%q, want %q", string(gotA), "two\\n")
	}
	gotB, err := os.ReadFile(filepath.Join(ens.Workspace.RunWorkDir, "b.txt"))
	if err != nil {
		t.Fatalf("read ws b.txt: %v", err)
	}
	if string(gotB) != "untracked\n" {
		t.Fatalf("ws b.txt=%q, want %q", string(gotB), "untracked\\n")
	}
}

func TestService_EnsureForTask_GitInitWithoutCommits_FallsBackToCopyWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	repo := t.TempDir()
	runGit(t, repo, "init")

	svc := NewService(store, Options{Retain: 5})
	task := tasks.Task{
		ID:             "t-3",
		ConversationID: uuid.NewString(),
		WorkDir:        repo,
	}
	ens, err := svc.EnsureForTask(ctx, task)
	if err != nil {
		t.Fatalf("EnsureForTask: %v", err)
	}
	if ens.Workspace.Kind != "copy" {
		t.Fatalf("kind=%q, want %q", ens.Workspace.Kind, "copy")
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
