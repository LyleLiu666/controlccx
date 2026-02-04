package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

func TestAPI_CreateTask_RejectsWhenWorkdirBusy(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:    taskStore,
		Workers:  nil,
		Observer: &observer.Service{Store: taskStore, Chat: chatStore},
		Chat:     chatStore,
		Hub:      hub,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	}
	buf, _ := json.Marshal(body)

	req1, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request1: %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "key-a")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("do1: %v", err)
	}
	var created tasks.Task
	if err := json.NewDecoder(res1.Body).Decode(&created); err != nil {
		res1.Body.Close()
		t.Fatalf("decode1: %v", err)
	}
	res1.Body.Close()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("status1=%d, want 200", res1.StatusCode)
	}
	if created.ID == "" {
		t.Fatalf("expected created id set")
	}

	req2, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request2: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "key-b")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do2: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("status2=%d, want 409", res2.StatusCode)
	}
	var errBody struct {
		Error          string `json:"error"`
		Message        string `json:"message"`
		WorkDir        string `json:"workdir"`
		ExistingTaskID string `json:"existing_task_id"`
		ExistingStatus string `json:"existing_status"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode2: %v", err)
	}
	if errBody.Error != "workdir_busy" {
		t.Fatalf("error=%q, want workdir_busy", errBody.Error)
	}
	if errBody.Message == "" {
		t.Fatalf("expected message non-empty")
	}
	if errBody.WorkDir == "" {
		t.Fatalf("expected workdir non-empty")
	}
	if errBody.ExistingTaskID != created.ID {
		t.Fatalf("existing_task_id=%q, want %q", errBody.ExistingTaskID, created.ID)
	}
	if errBody.ExistingStatus == "" {
		t.Fatalf("expected existing_status non-empty")
	}
}

func TestAPI_CreateTask_WorktreeStrategy_CreatesIsolatedWorkdir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	repo := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// Uncommitted changes that should be copied into the worktree.
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:    taskStore,
		Workers:  nil,
		Observer: &observer.Service{Store: taskStore, Chat: chatStore},
		Chat:     chatStore,
		Hub:      hub,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body1 := tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    repo,
	}
	buf1, _ := json.Marshal(body1)
	req1, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf1))
	if err != nil {
		t.Fatalf("new request1: %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "key-a")
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("do1: %v", err)
	}
	if res1.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res1.Body)
		res1.Body.Close()
		t.Fatalf("status1=%d, want 200; body=%s", res1.StatusCode, string(b))
	}
	res1.Body.Close()

	body2 := tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		WorkDirStrategy: "worktree",
		Prompt:          "echo world",
		WorkDir:         repo,
	}
	buf2, _ := json.Marshal(body2)
	req2, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks", bytes.NewReader(buf2))
	if err != nil {
		t.Fatalf("new request2: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "key-b")
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do2: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("status2=%d, want 200; body=%s", res2.StatusCode, string(b))
	}

	var created tasks.Task
	if err := json.NewDecoder(res2.Body).Decode(&created); err != nil {
		t.Fatalf("decode2: %v", err)
	}

	if strings.TrimSpace(created.ConversationID) == "" {
		t.Fatalf("expected conversation_id set")
	}
	if created.WorkDir == repo {
		t.Fatalf("expected workdir to differ from base repo")
	}
	if !strings.Contains(created.WorkDir, filepath.Join(repo, ".ccx", "worktrees")) {
		t.Fatalf("workdir=%q, want contains .ccx/worktrees under repo", created.WorkDir)
	}
	gotBase := created.BaseWorkDir
	wantBase := repo
	if v, err := filepath.EvalSymlinks(gotBase); err == nil {
		gotBase = v
	}
	if v, err := filepath.EvalSymlinks(wantBase); err == nil {
		wantBase = v
	}
	if gotBase != wantBase {
		t.Fatalf("base_workdir=%q, want %q", created.BaseWorkDir, repo)
	}
	if created.WorktreeDir != created.WorkDir {
		t.Fatalf("worktree_dir=%q, want %q", created.WorktreeDir, created.WorkDir)
	}
	if !strings.HasPrefix(created.WorktreeBranch, "ccx/") {
		t.Fatalf("worktree_branch=%q, want starts with ccx/", created.WorktreeBranch)
	}

	gotA, err := os.ReadFile(filepath.Join(created.WorkDir, "a.txt"))
	if err != nil {
		t.Fatalf("read worktree a.txt: %v", err)
	}
	if string(gotA) != "two\n" {
		t.Fatalf("worktree a.txt=%q, want %q", string(gotA), "two\n")
	}
	gotB, err := os.ReadFile(filepath.Join(created.WorkDir, "b.txt"))
	if err != nil {
		t.Fatalf("read worktree b.txt: %v", err)
	}
	if string(gotB) != "untracked\n" {
		t.Fatalf("worktree b.txt=%q, want %q", string(gotB), "untracked\n")
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
