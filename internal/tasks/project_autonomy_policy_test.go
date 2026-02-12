package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_ProjectAutonomyPolicy_DefaultAndUpsert(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	pk := NormalizeProjectKey(filepath.Join(t.TempDir(), "proj-a"))

	def, err := store.GetProjectAutonomyPolicy(ctx, pk)
	if err != nil {
		t.Fatalf("get default policy: %v", err)
	}
	if def.Mode != AutonomyModeGraded {
		t.Fatalf("default mode=%q, want %q", def.Mode, AutonomyModeGraded)
	}

	t0 := time.Date(2026, 2, 12, 14, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }
	updated, err := store.UpsertProjectAutonomyPolicy(ctx, pk, AutonomyModeMax)
	if err != nil {
		t.Fatalf("upsert policy: %v", err)
	}
	if updated.Mode != AutonomyModeMax {
		t.Fatalf("mode=%q, want %q", updated.Mode, AutonomyModeMax)
	}
	if updated.UpdatedAt != t0 {
		t.Fatalf("updated_at=%s, want %s", updated.UpdatedAt, t0)
	}
}

func TestStore_ProjectAutonomyPolicy_Validation(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	if _, err := store.UpsertProjectAutonomyPolicy(ctx, "", "max"); err == nil {
		t.Fatalf("expected project_key validation error")
	}
	if _, err := store.UpsertProjectAutonomyPolicy(ctx, filepath.Join(t.TempDir(), "proj-b"), "unsafe"); err == nil {
		t.Fatalf("expected mode validation error")
	}
}

