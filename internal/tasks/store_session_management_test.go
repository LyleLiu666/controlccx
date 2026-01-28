package tasks

import (
	"context"
	"path/filepath"
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

	// Before the task has a session_id, its session key should be t:<task_id>.
	initialKey := SessionKey(task.ID, "")
	if err := store.RenameSession(ctx, initialKey, "My Session"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionTitle != "My Session" {
		t.Fatalf("session_title=%q, want %q", got.SessionTitle, "My Session")
	}

	// Once session_id is set, the key becomes s:<session_id> and metadata should migrate.
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
	if got.SessionTitle != "My Session" {
		t.Fatalf("session_title=%q, want %q after migrate", got.SessionTitle, "My Session")
	}

	// Soft delete hides the session by default.
	newKey := SessionKey(task.ID, "sess-123")
	if err := store.DeleteSession(ctx, newKey); err != nil {
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

