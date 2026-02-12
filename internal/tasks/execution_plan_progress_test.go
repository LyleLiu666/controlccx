package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_ExecutionPlanProgress_AppendAndList(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	t0 := time.Date(2026, 2, 12, 13, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }
	if _, err := store.AppendExecutionPlanProgress(ctx, AppendExecutionPlanProgressInput{
		Key:       "c:conv-progress",
		Iteration: 1,
		Action:    "resume_run",
		Status:    "running",
		Summary:   "step 1 started",
	}); err != nil {
		t.Fatalf("append #1: %v", err)
	}

	t1 := t0.Add(1 * time.Minute)
	store.now = func() time.Time { return t1 }
	if _, err := store.AppendExecutionPlanProgress(ctx, AppendExecutionPlanProgressInput{
		Key:       "c:conv-progress",
		Iteration: 1,
		Action:    "wait_in_flight",
		Status:    "waiting",
		Summary:   "waiting for current run",
	}); err != nil {
		t.Fatalf("append #2: %v", err)
	}

	list, err := store.ListExecutionPlanProgress(ctx, "c:conv-progress", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d, want 2", len(list))
	}
	if list[0].Status != "waiting" || list[1].Status != "running" {
		t.Fatalf("unexpected order/status: %+v", list)
	}
}
