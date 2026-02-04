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
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
	"github.com/google/uuid"
)

func TestAPI_CreateTask_WorktreeRejectsNonUUIDConversationID(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	apiSvc := &API{Tasks: tasks.NewStore(conn)}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body := tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		Prompt:          "echo hello",
		WorkDir:         t.TempDir(),
		WorkDirStrategy: "worktree",
		ConversationID:  "not-a-uuid",
	}
	buf, _ := json.Marshal(body)
	res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d want 400; body=%s", res.StatusCode, string(b))
	}
}

func TestAPI_CreateTask_WorktreeUntrackedTooLargeRequiresConfirmThenSkipProceeds(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	apiSvc := &API{Tasks: tasks.NewStore(conn)}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "ccx@example.com")
	runGit(t, repo, "config", "user.name", "ccx")
	mustWriteFile(t, filepath.Join(repo, "a.txt"), "one\n")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// Exceed default max_bytes (20 MB) without writing huge data by truncating.
	const maxBytes = int64(20 * 1024 * 1024)
	f, err := os.Create(filepath.Join(repo, "big.bin"))
	if err != nil {
		t.Fatalf("create big: %v", err)
	}
	if err := f.Truncate(maxBytes + 1); err != nil {
		_ = f.Close()
		t.Fatalf("truncate: %v", err)
	}
	_ = f.Close()

	req := tasks.CreateTaskInput{
		WorkerType:      tasks.WorkerExec,
		Mode:            tasks.ModeNew,
		Prompt:          "echo hello",
		WorkDir:         repo,
		WorkDirStrategy: "worktree",
	}
	buf, _ := json.Marshal(req)
	res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d want 422; body=%s", res.StatusCode, string(b))
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "worktree_untracked_too_large" {
		t.Fatalf("unexpected error=%v", payload["error"])
	}
	cid := payload["conversation_id"]
	if _, err := uuid.Parse(anyString(cid)); err != nil {
		t.Fatalf("expected conversation_id uuid, got %v (err=%v)", cid, err)
	}

	// Retry with explicit skip.
	req2 := tasks.CreateTaskInput{
		WorkerType:         tasks.WorkerExec,
		Mode:               tasks.ModeNew,
		Prompt:             "echo hello",
		WorkDir:            repo,
		WorkDirStrategy:    "worktree",
		ConversationID:     anyString(cid),
		WorktreeUntracked:  "skip",
	}
	buf2, _ := json.Marshal(req2)
	res2, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf2))
	if err != nil {
		t.Fatalf("post retry: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("status=%d want 200; body=%s", res2.StatusCode, string(b))
	}
	var created tasks.Task
	if err := json.NewDecoder(res2.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.WorkDirStrategy != "worktree" || created.WorkDir == "" || created.WorktreeDir == "" || created.WorktreeBranch == "" {
		t.Fatalf("unexpected created worktree meta: %+v", created)
	}
	if _, err := os.Stat(filepath.Join(created.WorktreeDir, "big.bin")); err == nil {
		t.Fatalf("expected big.bin not copied when worktree_untracked=skip")
	}
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
