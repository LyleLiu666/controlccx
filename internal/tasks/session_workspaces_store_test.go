package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_UpsertSessionWorkspace_RoundTripsAndPreservesCreatedAt(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	t0 := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return t0 }

	in := UpsertSessionWorkspaceInput{
		Key:         "c:2a15e00c-e1ff-4834-974e-61176e720568",
		Kind:        "git-worktree",
		BaseWorkDir: "/tmp/base",
		RepoRoot:    "/tmp/repo",
		RunRoot:     "/tmp/repo/.ccx/worktrees/2a15e00c-e1ff-4834-974e-61176e720568/ws",
		RunWorkDir:  "/tmp/repo/.ccx/worktrees/2a15e00c-e1ff-4834-974e-61176e720568/ws",
		BaseBranch:  "main",
		WorkBranch:  "ccx/2a15e00c-e1ff-4834-974e-61176e720568/ws",
		Status:      "active",
	}
	got, err := store.UpsertSessionWorkspace(ctx, in)
	if err != nil {
		t.Fatalf("UpsertSessionWorkspace: %v", err)
	}
	if got.Key != in.Key || got.Kind != in.Kind || got.RunWorkDir != in.RunWorkDir || got.Status != in.Status {
		t.Fatalf("unexpected workspace: %+v", got)
	}
	if got.CreatedAt.UTC() != t0 || got.UpdatedAt.UTC() != t0 {
		t.Fatalf("unexpected times: created=%s updated=%s", got.CreatedAt.UTC(), got.UpdatedAt.UTC())
	}

	// Update should preserve created_at but move updated_at.
	t1 := t0.Add(3 * time.Hour)
	store.now = func() time.Time { return t1 }
	in.Status = "merged"
	got2, err := store.UpsertSessionWorkspace(ctx, in)
	if err != nil {
		t.Fatalf("UpsertSessionWorkspace(2): %v", err)
	}
	if got2.Status != "merged" {
		t.Fatalf("status=%q, want %q", got2.Status, "merged")
	}
	if got2.CreatedAt.UTC() != t0 {
		t.Fatalf("created_at=%s, want %s", got2.CreatedAt.UTC(), t0)
	}
	if got2.UpdatedAt.UTC() != t1 {
		t.Fatalf("updated_at=%s, want %s", got2.UpdatedAt.UTC(), t1)
	}
}
