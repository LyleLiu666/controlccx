package tasks

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_SessionRenameDeleteAndMigration(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	fixedNow := time.Date(2026, 1, 28, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixedNow }

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if strings.TrimSpace(task.ConversationID) == "" {
		t.Fatalf("conversation_id is required")
	}

	// Session key is conversation-scoped and stable (not tied to provider session_id).
	key := SessionKeyForTask(task)
	if err := store.RenameSession(ctx, key, "My Session"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionTitle != "My Session" {
		t.Fatalf("session_title=%q, want %q", got.SessionTitle, "My Session")
	}

	// Once session_id is set, the session key should remain stable (conversation-scoped).
	if err := store.FinishTask(ctx, task.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		SessionID:  "sess-123",
		FinishedAt: fixedNow,
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, _ = store.GetTask(ctx, task.ID)
	if got.SessionID != "sess-123" {
		t.Fatalf("session_id=%q, want sess-123", got.SessionID)
	}
	if SessionKeyForTask(got) != key {
		t.Fatalf("session key changed: got=%q want=%q", SessionKeyForTask(got), key)
	}
	if got.SessionTitle != "My Session" {
		t.Fatalf("session_title=%q, want %q", got.SessionTitle, "My Session")
	}

	// Soft delete hides the session by default.
	if err := store.DeleteSession(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list, err := store.ListTasks(ctx, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len(list)=%d, want 0 (deleted session hidden)", len(list))
	}

	listIncl, err := store.ListTasksWithOptions(ctx, 100, ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list include_deleted: %v", err)
	}
	if len(listIncl) != 1 {
		t.Fatalf("len(listIncl)=%d, want 1", len(listIncl))
	}
	if listIncl[0].SessionDeletedAt == nil {
		t.Fatalf("expected session_deleted_at set")
	}
}
