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

func TestService_CopyWorkspace_MountsVenvSymlink(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)

	baseDir := t.TempDir()
	mustMkdir(t, filepath.Join(baseDir, ".venv", "bin"))
	if err := os.WriteFile(filepath.Join(baseDir, ".venv", "bin", "python"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write venv python: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}

	svc := NewService(store)
	ws, err := svc.EnsureForTask(ctx, tasks.Task{ID: "task-venv-copy", WorkDir: baseDir})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ws.Kind != tasks.WorkspaceKindCopy {
		t.Fatalf("kind=%q, want %q", ws.Kind, tasks.WorkspaceKindCopy)
	}

	dst := filepath.Join(ws.RunWorkDir, ".venv")
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat mounted venv: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s, mode=%v", dst, fi.Mode())
	}
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	got, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval target: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(baseDir, ".venv"))
	if err != nil {
		t.Fatalf("eval want: %v", err)
	}
	if got != want {
		t.Fatalf("link target=%q (resolved=%q), want %q", target, got, want)
	}
}

func TestService_GitWorktree_MountsVenvSymlink(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".venv/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	run("add", ".gitignore", "a.txt")
	run("commit", "-q", "-m", "init")

	mustMkdir(t, filepath.Join(repo, ".venv", "bin"))
	if err := os.WriteFile(filepath.Join(repo, ".venv", "bin", "python"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write venv python: %v", err)
	}

	svc := NewService(store)
	ws, err := svc.EnsureForTask(ctx, tasks.Task{ID: "task-venv-wt", WorkDir: repo})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if ws.Kind != tasks.WorkspaceKindGitWorktree {
		t.Fatalf("kind=%q, want %q", ws.Kind, tasks.WorkspaceKindGitWorktree)
	}

	dst := filepath.Join(ws.RunWorkDir, ".venv")
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat mounted venv: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s, mode=%v", dst, fi.Mode())
	}
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	got, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("eval target: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(repo, ".venv"))
	if err != nil {
		t.Fatalf("eval want: %v", err)
	}
	if got != want {
		t.Fatalf("link target=%q (resolved=%q), want %q", target, got, want)
	}
}

func TestService_Resume_ReusesSessionWorkspaceAfterSessionIDSet(t *testing.T) {
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

	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    baseDir,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := NewService(store)
	ws1, err := svc.EnsureForTask(ctx, first)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	wantKey := tasks.SessionKeyForTask(first)
	if ws1.Key != wantKey {
		t.Fatalf("ws1.key=%q, want %q", ws1.Key, wantKey)
	}

	if err := store.SetSessionID(ctx, first.ID, "sess-1"); err != nil {
		t.Fatalf("set session id: %v", err)
	}

	if ws2, ok, err := store.GetSessionWorkspace(ctx, wantKey); err != nil || !ok {
		t.Fatalf("expected workspace key stable; ok=%v err=%v", ok, err)
	} else if ws2.WorkspaceID != ws1.WorkspaceID {
		t.Fatalf("workspace_id=%q, want %q", ws2.WorkspaceID, ws1.WorkspaceID)
	}

	resume, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "y",
		WorkDir:    baseDir,
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create resume task: %v", err)
	}
	if strings.TrimSpace(resume.ConversationID) != strings.TrimSpace(first.ConversationID) {
		t.Fatalf("resume conversation_id=%q, want %q", resume.ConversationID, first.ConversationID)
	}
	ws2, err := svc.EnsureForTask(ctx, resume)
	if err != nil {
		t.Fatalf("ensure resume: %v", err)
	}
	if ws2.WorkspaceID != ws1.WorkspaceID {
		t.Fatalf("resume workspace_id=%q, want %q", ws2.WorkspaceID, ws1.WorkspaceID)
	}
	if ws2.RunWorkDir != ws1.RunWorkDir {
		t.Fatalf("resume run_workdir=%q, want %q", ws2.RunWorkDir, ws1.RunWorkDir)
	}
}

func TestService_Resume_RecoversLegacySessionWorkspaceKey(t *testing.T) {
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

	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    baseDir,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	svc := NewService(store)
	ws1, err := svc.EnsureForTask(ctx, first)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	const sid = "sess-legacy"
	if err := store.SetSessionID(ctx, first.ID, sid); err != nil {
		t.Fatalf("set session id: %v", err)
	}

	// Simulate legacy DB: workspace stored under s:<session_id> rather than conversation key.
	desiredKey := tasks.SessionKeyForTask(first)
	legacySessionKey := tasks.SessionKey("", sid)
	if _, err := conn.ExecContext(ctx, `UPDATE session_workspaces SET key = ? WHERE key = ?;`, legacySessionKey, desiredKey); err != nil {
		t.Fatalf("force legacy workspace key: %v", err)
	}

	resume, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeResume,
		Prompt:     "y",
		WorkDir:    baseDir,
		SessionID:  sid,
	})
	if err != nil {
		t.Fatalf("create resume task: %v", err)
	}
	ws2, err := svc.EnsureForTask(ctx, resume)
	if err != nil {
		t.Fatalf("ensure resume: %v", err)
	}
	if ws2.WorkspaceID != ws1.WorkspaceID {
		t.Fatalf("resume workspace_id=%q, want %q", ws2.WorkspaceID, ws1.WorkspaceID)
	}
	if ws2.RunWorkDir != ws1.RunWorkDir {
		t.Fatalf("resume run_workdir=%q, want %q", ws2.RunWorkDir, ws1.RunWorkDir)
	}
	if ws2.Key != desiredKey {
		t.Fatalf("resume key=%q, want %q", ws2.Key, desiredKey)
	}

	// Legacy key should be migrated back to the conversation key.
	if _, ok, err := store.GetSessionWorkspace(ctx, legacySessionKey); err != nil || ok {
		t.Fatalf("legacy session workspace key should be migrated; ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetSessionWorkspace(ctx, desiredKey); err != nil || !ok {
		t.Fatalf("expected conversation-key workspace mapping after recovery; ok=%v err=%v", ok, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
