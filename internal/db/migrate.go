package db

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

func Migrate(ctx context.Context, conn *sql.DB) error {
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version;").Scan(&current); err != nil {
		return fmt.Errorf("db: read user_version: %w", err)
	}
	if current != 0 && current != schemaVersion {
		return fmt.Errorf("db: unsupported schema version %d (expected %d)", current, schemaVersion)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin migrate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var stmts []string
	if current == 0 {
		stmts = append(stmts,
			`CREATE TABLE IF NOT EXISTS tasks (
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
			);`,
			`CREATE TABLE IF NOT EXISTS logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				task_id TEXT NOT NULL,
				ts INTEGER NOT NULL,
				stream TEXT NOT NULL,
				message TEXT NOT NULL,
				FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
			);`,
			`CREATE INDEX IF NOT EXISTS idx_logs_task_id_id ON logs(task_id, id);`,
			`CREATE TABLE IF NOT EXISTS chat_messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				ts INTEGER NOT NULL,
				role TEXT NOT NULL,
				content TEXT NOT NULL
			);`,
			fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion),
		)
	}

	// Additive tables (safe to run on existing DBs with user_version==schemaVersion).
	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS task_run_options (
			task_id TEXT PRIMARY KEY,
			unsafe_automation INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS session_meta (
			key TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			deleted_at INTEGER,
			updated_at INTEGER NOT NULL DEFAULT 0
		);`,
	)

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("db: migrate stmt failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: migrate commit: %w", err)
	}
	return nil
}
