package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_MissionContract_UpsertAndGet(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	t0 := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }

	_, ok, err := store.GetMissionContract(ctx, "c:conv-1")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if ok {
		t.Fatalf("expected missing mission contract")
	}

	created, err := store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  "c:conv-1",
		Goal: "Implement safe autonomous delivery for one project",
		Constraints: []string{
			"must run tests before commit",
			"no destructive git commands",
		},
		AcceptanceCriteria: []string{
			"narrow tests pass",
			"full regression pass",
		},
		NonGoals: []string{
			"rewrite all frontend pages",
		},
	})
	if err != nil {
		t.Fatalf("upsert create: %v", err)
	}
	if created.Revision != 1 {
		t.Fatalf("revision=%d, want 1", created.Revision)
	}
	if created.Goal != "Implement safe autonomous delivery for one project" {
		t.Fatalf("goal=%q", created.Goal)
	}
	if len(created.Constraints) != 2 {
		t.Fatalf("constraints=%v", created.Constraints)
	}
	if len(created.AcceptanceCriteria) != 2 {
		t.Fatalf("acceptance_criteria=%v", created.AcceptanceCriteria)
	}
	if len(created.NonGoals) != 1 {
		t.Fatalf("non_goals=%v", created.NonGoals)
	}
	if created.CreatedAt != t0 || created.UpdatedAt != t0 {
		t.Fatalf("timestamps created=%s updated=%s, want %s", created.CreatedAt, created.UpdatedAt, t0)
	}

	t1 := t0.Add(10 * time.Minute)
	store.now = func() time.Time { return t1 }
	updated, err := store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  "c:conv-1",
		Goal: "Implement safe autonomous delivery for multiple projects",
		Constraints: []string{
			"must run tests before commit",
		},
		AcceptanceCriteria: []string{
			"full regression pass",
			"openspec plan check passes",
		},
		NonGoals: []string{
			"build a brand-new UI framework",
		},
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if updated.Revision != 2 {
		t.Fatalf("revision=%d, want 2", updated.Revision)
	}
	if updated.CreatedAt != t0 {
		t.Fatalf("created_at=%s, want %s", updated.CreatedAt, t0)
	}
	if updated.UpdatedAt != t1 {
		t.Fatalf("updated_at=%s, want %s", updated.UpdatedAt, t1)
	}

	got, ok, err := store.GetMissionContract(ctx, "c:conv-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected mission contract")
	}
	if got.Revision != 2 {
		t.Fatalf("get revision=%d, want 2", got.Revision)
	}
	if got.Goal != updated.Goal {
		t.Fatalf("get goal=%q, want %q", got.Goal, updated.Goal)
	}
}

func TestStore_MissionContract_Validation(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	_, err = store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  "",
		Goal: "Ship safely",
	})
	if err == nil {
		t.Fatalf("expected key validation error")
	}
	_, err = store.UpsertMissionContract(ctx, UpsertMissionContractInput{
		Key:  "c:conv-2",
		Goal: "   ",
	})
	if err == nil {
		t.Fatalf("expected goal validation error")
	}
}
