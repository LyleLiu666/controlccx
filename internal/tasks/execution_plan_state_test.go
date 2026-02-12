package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_ExecutionPlanState_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	t0 := time.Date(2026, 2, 12, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }

	_, ok, err := store.GetExecutionPlanState(ctx, "c:conv-plan")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if ok {
		t.Fatalf("expected missing state")
	}

	created, err := store.UpsertExecutionPlanState(ctx, UpsertExecutionPlanStateInput{
		Key:             "c:conv-plan",
		MissionRevision: 2,
		PlanJSON:        `{"version":"v1","steps":["a","b"]}`,
		Iteration:       1,
		LastAction:      "resume_run",
		LastTaskID:      "task-1",
		Status:          "running",
	})
	if err != nil {
		t.Fatalf("upsert create: %v", err)
	}
	if created.Iteration != 1 {
		t.Fatalf("iteration=%d, want 1", created.Iteration)
	}
	if created.UpdatedAt != t0 {
		t.Fatalf("updated_at=%s, want %s", created.UpdatedAt, t0)
	}

	t1 := t0.Add(3 * time.Minute)
	store.now = func() time.Time { return t1 }
	updated, err := store.UpsertExecutionPlanState(ctx, UpsertExecutionPlanStateInput{
		Key:             "c:conv-plan",
		MissionRevision: 2,
		PlanJSON:        `{"version":"v1","steps":["a","b"]}`,
		Iteration:       2,
		LastAction:      "start_run",
		LastTaskID:      "task-2",
		Status:          "queued",
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if updated.Iteration != 2 {
		t.Fatalf("iteration=%d, want 2", updated.Iteration)
	}
	if updated.UpdatedAt != t1 {
		t.Fatalf("updated_at=%s, want %s", updated.UpdatedAt, t1)
	}
}

func TestStore_ExecutionPlanState_Validation(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	_, err = store.UpsertExecutionPlanState(ctx, UpsertExecutionPlanStateInput{
		Key: "",
	})
	if err == nil {
		t.Fatalf("expected key validation error")
	}
}
