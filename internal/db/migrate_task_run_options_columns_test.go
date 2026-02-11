package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrate_TaskRunOptions_AddColumnsIfMissing(t *testing.T) {
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

	// Simulate an existing DB at schemaVersion with the legacy task_run_options schema.
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
		CREATE TABLE IF NOT EXISTS task_run_options (
			task_id TEXT PRIMARY KEY,
			unsafe_automation INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
	`); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 1;"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	type col struct {
		Name string
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(task_run_options);")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })

	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	// New columns added by this change.
	for _, want := range []string{
		"safety_preset",
		"task_intent",
		"network_tier",
		"codex_sandbox",
		"codex_approval_policy",
		"codex_search",
		"claude_permission_mode",
		"claude_sandbox",
		"claude_webfetch_domains_json",
	} {
		if !cols[want] {
			t.Fatalf("expected column %q in task_run_options; cols=%v", want, cols)
		}
	}
}
