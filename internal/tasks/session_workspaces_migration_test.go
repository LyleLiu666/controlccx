package tasks

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_SetSessionID_MigratesSessionWorkspacesKeyForLegacyTasks(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	store.now = func() time.Time { return time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC) }

	nowMs := toMillis(store.now().UTC())
	baseDir := t.TempDir()

	const taskID = "task-legacy"
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO tasks (
			id, worker_type, mode, status, prompt, workdir,
			session_id, conversation_id,
			created_at, updated_at, stderr_count, keyword_count, score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0);
	`, taskID, string(WorkerExec), string(ModeNew), string(StatusSucceeded), "echo ok", baseDir,
		"", "",
		nowMs-1000, nowMs-1000,
	); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}

	fromKey := SessionKey(taskID, "")
	toKey := SessionKey(taskID, "sess-1")

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO session_workspaces (
			key, kind, base_workdir, run_root, run_workdir, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`, fromKey, "copy", baseDir, filepath.Join(baseDir, ".ccx", "workspaces", "legacy"), filepath.Join(baseDir, ".ccx", "workspaces", "legacy"), "active", nowMs-1000, nowMs-1000); err != nil {
		t.Fatalf("insert session_workspaces: %v", err)
	}

	if err := store.SetSessionID(ctx, taskID, "sess-1"); err != nil {
		t.Fatalf("SetSessionID: %v", err)
	}

	var gotKey string
	var gotUpdatedAt int64
	if err := conn.QueryRowContext(ctx, `SELECT key, updated_at FROM session_workspaces WHERE key = ?;`, toKey).Scan(&gotKey, &gotUpdatedAt); err != nil {
		t.Fatalf("expected migrated session_workspaces: %v", err)
	}
	if gotKey != toKey {
		t.Fatalf("migrated key=%q, want %q", gotKey, toKey)
	}
	if gotUpdatedAt != nowMs {
		t.Fatalf("migrated updated_at=%d, want %d", gotUpdatedAt, nowMs)
	}
	if err := conn.QueryRowContext(ctx, `SELECT key FROM session_workspaces WHERE key = ?;`, fromKey).Scan(new(string)); err == nil {
		t.Fatalf("expected legacy session_workspaces key removed")
	}
}

