package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrate_SecretaryCompressions_AddConversationIDBackfillAndIndex(t *testing.T) {
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

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			worker_type TEXT NOT NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			prompt TEXT NOT NULL,
			workdir TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			conversation_id TEXT NOT NULL DEFAULT '',
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
		CREATE TABLE IF NOT EXISTS secretary_compressions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			cursor_before INTEGER NOT NULL,
			cursor_after INTEGER NOT NULL,
			keep_from INTEGER NOT NULL,
			summary TEXT NOT NULL,
			backend TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO secretary_compressions(ts, cursor_before, cursor_after, keep_from, summary, backend, error)
		VALUES (1001, 0, 1, 2, 'legacy', 'simple-http', '');
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 1;"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(secretary_compressions);")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if !cols["conversation_id"] {
		t.Fatalf("expected conversation_id column; cols=%v", cols)
	}

	var gotConversationID string
	if err := conn.QueryRowContext(ctx, `
		SELECT conversation_id
		FROM secretary_compressions
		WHERE summary='legacy'
	`).Scan(&gotConversationID); err != nil {
		t.Fatalf("query backfilled row: %v", err)
	}
	if gotConversationID != "__global__" {
		t.Fatalf("legacy row conversation_id=%q want %q", gotConversationID, "__global__")
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO secretary_compressions(ts, cursor_before, cursor_after, keep_from, summary, backend, error)
		VALUES (1002, 1, 2, 3, 'new', 'simple-http', '');
	`); err != nil {
		t.Fatalf("insert new row: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `
		SELECT conversation_id
		FROM secretary_compressions
		WHERE summary='new'
	`).Scan(&gotConversationID); err != nil {
		t.Fatalf("query new row: %v", err)
	}
	if gotConversationID != "__global__" {
		t.Fatalf("new row conversation_id=%q want %q", gotConversationID, "__global__")
	}

	var indexName string
	if err := conn.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='index' AND name='idx_secretary_compressions_conversation_id_id';
	`).Scan(&indexName); err != nil {
		t.Fatalf("expected idx_secretary_compressions_conversation_id_id index: %v", err)
	}
}
