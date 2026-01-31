package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrate_AdditiveTables_AllowedAtSchemaVersion(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}

	// Simulate an existing DB at schemaVersion with only base tables.
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			worker_type TEXT NOT NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			prompt TEXT NOT NULL,
			workdir TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			started_at INTEGER,
			finished_at INTEGER,
			exit_code INTEGER,
			error TEXT NOT NULL DEFAULT '',
			warning TEXT NOT NULL DEFAULT '',
			stderr_count INTEGER NOT NULL DEFAULT 0,
			keyword_count INTEGER NOT NULL DEFAULT 0,
			score INTEGER NOT NULL DEFAULT 0
		);
	`); err != nil {
		t.Fatalf("create base tasks table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 1;"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var name string
	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='task_run_options';
	`).Scan(&name); err != nil {
		t.Fatalf("expected task_run_options table: %v", err)
	}

	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='session_meta';
	`).Scan(&name); err != nil {
		t.Fatalf("expected session_meta table: %v", err)
	}

	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='task_invocations';
	`).Scan(&name); err != nil {
		t.Fatalf("expected task_invocations table: %v", err)
	}

	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='acceptance_states';
	`).Scan(&name); err != nil {
		t.Fatalf("expected acceptance_states table: %v", err)
	}

	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='session_workspaces';
	`).Scan(&name); err != nil {
		t.Fatalf("expected session_workspaces table: %v", err)
	}
}
