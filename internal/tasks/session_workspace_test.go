package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_SessionWorkspace_CRUD(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	fixedNow := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixedNow }

	ws, err := store.UpsertSessionWorkspace(ctx, SessionWorkspace{
		Key:         "t:task-1",
		WorkspaceID: "ws-1",
		Kind:        WorkspaceKindCopy,
		BaseWorkDir: "/base",
		RunRoot:     "/run",
		RunWorkDir:  "/run",
		Status:      WorkspaceStatusActive,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if ws.Key != "t:task-1" {
		t.Fatalf("key=%q", ws.Key)
	}
	if ws.Status != WorkspaceStatusActive {
		t.Fatalf("status=%q", ws.Status)
	}
	if ws.CreatedAt.IsZero() || ws.UpdatedAt.IsZero() {
		t.Fatalf("expected created/updated timestamps set")
	}

	got, ok, err := store.GetSessionWorkspace(ctx, "t:task-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected workspace")
	}
	if got.WorkspaceID != "ws-1" {
		t.Fatalf("workspace_id=%q", got.WorkspaceID)
	}

	store.now = func() time.Time { return fixedNow.Add(2 * time.Hour) }
	if err := store.SetSessionWorkspaceStatus(ctx, "t:task-1", WorkspaceStatusMerged); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, ok, err = store.GetSessionWorkspace(ctx, "t:task-1")
	if err != nil || !ok {
		t.Fatalf("get after status: ok=%v err=%v", ok, err)
	}
	if got.Status != WorkspaceStatusMerged {
		t.Fatalf("status=%q", got.Status)
	}
	if !got.UpdatedAt.After(ws.UpdatedAt) {
		t.Fatalf("expected updated_at to advance")
	}

	list, err := store.ListSessionWorkspaces(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len=%d", len(list))
	}

	if err := store.DeleteSessionWorkspace(ctx, "t:task-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, err = store.GetSessionWorkspace(ctx, "t:task-1")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected workspace deleted")
	}
}
