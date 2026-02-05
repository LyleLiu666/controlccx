package tasks

import (
	"context"
	"path/filepath"
	"strings"
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

func TestStore_UpsertSessionWorkspace_LegacySchemaWithWorkspaceID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.ExecContext(ctx, `DROP TABLE session_workspaces;`); err != nil {
		t.Fatalf("drop session_workspaces: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE session_workspaces (
			key TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			base_workdir TEXT NOT NULL,
			repo_root TEXT NOT NULL DEFAULT '',
			run_root TEXT NOT NULL,
			run_workdir TEXT NOT NULL,
			base_branch TEXT NOT NULL DEFAULT '',
			work_branch TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
	`); err != nil {
		t.Fatalf("create legacy session_workspaces: %v", err)
	}

	store := NewStore(conn)
	store.now = func() time.Time { return time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC) }

	const wantWorkspaceID = "legacy-workspace-id"
	in := UpsertSessionWorkspaceInput{
		Key:         "c:2a15e00c-e1ff-4834-974e-61176e720568",
		Kind:        "copy",
		BaseWorkDir: "/tmp/base",
		RepoRoot:    "",
		RunRoot:     "/tmp/base/.ccx/workspaces/" + wantWorkspaceID + "/copy",
		RunWorkDir:  "/tmp/base/.ccx/workspaces/" + wantWorkspaceID + "/copy",
		Status:      "active",
	}

	if _, err := store.UpsertSessionWorkspace(ctx, in); err != nil {
		t.Fatalf("UpsertSessionWorkspace: %v", err)
	}

	var gotWorkspaceID string
	if err := conn.QueryRowContext(ctx, `SELECT workspace_id FROM session_workspaces WHERE key = ?;`, in.Key).Scan(&gotWorkspaceID); err != nil {
		t.Fatalf("read workspace_id: %v", err)
	}
	if strings.TrimSpace(gotWorkspaceID) == "" {
		t.Fatalf("workspace_id is empty")
	}
	if gotWorkspaceID != wantWorkspaceID {
		t.Fatalf("workspace_id=%q, want %q", gotWorkspaceID, wantWorkspaceID)
	}
}
