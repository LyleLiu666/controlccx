package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_CountByStatus_RespectsDeletedSessions(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)

	t1, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "t1",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create t1: %v", err)
	}
	exit0 := 0
	if err := store.FinishTask(ctx, t1.ID, FinishTaskInput{
		Status:     StatusSucceeded,
		ExitCode:   &exit0,
		Error:      "",
		SessionID:  "sess-1",
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish t1: %v", err)
	}

	t2, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "t2",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create t2: %v", err)
	}
	exit1 := 1
	if err := store.FinishTask(ctx, t2.ID, FinishTaskInput{
		Status:     StatusFailed,
		ExitCode:   &exit1,
		Error:      "boom",
		SessionID:  "sess-2",
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish t2: %v", err)
	}

	_, err = store.CreateTask(ctx, CreateTaskInput{
		WorkerType: WorkerExec,
		Mode:       ModeNew,
		Prompt:     "t3",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create t3: %v", err)
	}

	if err := store.DeleteSession(ctx, ConversationKey(t2.ConversationID)); err != nil {
		t.Fatalf("delete session for t2: %v", err)
	}

	by, total, err := store.CountByStatus(ctx, ListTasksOptions{IncludeDeleted: false})
	if err != nil {
		t.Fatalf("count by status: %v", err)
	}
	if total != 2 {
		t.Fatalf("total=%d want %d", total, 2)
	}
	if by[StatusSucceeded] != 1 {
		t.Fatalf("succeeded=%d want %d", by[StatusSucceeded], 1)
	}
	if by[StatusQueued] != 1 {
		t.Fatalf("queued=%d want %d", by[StatusQueued], 1)
	}
	if by[StatusFailed] != 0 {
		t.Fatalf("failed=%d want %d", by[StatusFailed], 0)
	}

	by2, total2, err := store.CountByStatus(ctx, ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("count by status include_deleted: %v", err)
	}
	if total2 != 3 {
		t.Fatalf("total=%d want %d", total2, 3)
	}
	if by2[StatusFailed] != 1 {
		t.Fatalf("failed=%d want %d", by2[StatusFailed], 1)
	}
}
