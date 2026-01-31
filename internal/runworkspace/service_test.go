package runworkspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestService_CopyWorkspace_MergeAndConflicts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}

	svc := NewService(store)
	ws, err := svc.EnsureForTask(ctx, tasks.Task{ID: "task-1", WorkDir: baseDir})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ws.Kind != tasks.WorkspaceKindCopy {
		t.Fatalf("kind=%q, want %q", ws.Kind, tasks.WorkspaceKindCopy)
	}
	if _, err := os.Stat(filepath.Join(ws.RunRoot, copyManifestName)); err != nil {
		t.Fatalf("expected manifest: %v", err)
	}

	if err := os.WriteFile(filepath.Join(ws.RunWorkDir, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write run file: %v", err)
	}

	if err := svc.Merge(ctx, ws.Key); err != nil {
		t.Fatalf("merge: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(baseDir, "a.txt"))
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	if strings.TrimSpace(string(got)) != "changed" {
		t.Fatalf("base=%q, want %q", strings.TrimSpace(string(got)), "changed")
	}

	ws2, err := svc.EnsureForTask(ctx, tasks.Task{ID: "task-2", WorkDir: baseDir})
	if err != nil {
		t.Fatalf("ensure 2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "a.txt"), []byte("base edit\n"), 0o644); err != nil {
		t.Fatalf("write base edit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws2.RunWorkDir, "a.txt"), []byte("run edit\n"), 0o644); err != nil {
		t.Fatalf("write run edit: %v", err)
	}
	err = svc.Merge(ctx, ws2.Key)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
	if len(conflict.Conflicts) == 0 || conflict.Conflicts[0] != "a.txt" {
		t.Fatalf("conflicts=%v, want contains a.txt", conflict.Conflicts)
	}
}

func TestService_GitWorktree_Merge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
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
	run := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out))
	}

	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	svc := NewService(store)
	ws, err := svc.EnsureForTask(ctx, tasks.Task{ID: "task-git-1", WorkDir: repo})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ws.Kind != tasks.WorkspaceKindGitWorktree {
		t.Fatalf("kind=%q, want %q", ws.Kind, tasks.WorkspaceKindGitWorktree)
	}
	if _, err := os.Stat(filepath.Join(ws.RunWorkDir, "notes.txt")); err != nil {
		t.Fatalf("expected untracked copied: %v", err)
	}

	if err := os.WriteFile(filepath.Join(ws.RunWorkDir, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write run a.txt: %v", err)
	}

	if err := svc.Merge(ctx, ws.Key); err != nil {
		t.Fatalf("merge: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read base a.txt: %v", err)
	}
	if strings.TrimSpace(string(got)) != "changed" {
		t.Fatalf("base=%q, want %q", strings.TrimSpace(string(got)), "changed")
	}
}
