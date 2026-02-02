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
			safety_preset TEXT NOT NULL DEFAULT '',
			task_intent TEXT NOT NULL DEFAULT '',
			codex_sandbox TEXT NOT NULL DEFAULT '',
			codex_approval_policy TEXT NOT NULL DEFAULT '',
			codex_search INTEGER NOT NULL DEFAULT 0,
			claude_permission_mode TEXT NOT NULL DEFAULT '',
			claude_sandbox INTEGER NOT NULL DEFAULT 0,
			claude_webfetch_domains_json TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS task_invocations (
			task_id TEXT PRIMARY KEY,
			cmd TEXT NOT NULL,
			args_json TEXT NOT NULL,
			dir TEXT NOT NULL,
			env_keys_json TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS session_meta (
			key TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			deleted_at INTEGER,
			updated_at INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS session_workspaces (
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
		);`,
		`CREATE TABLE IF NOT EXISTS acceptance_states (
			session_key TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT '',
			iteration INTEGER NOT NULL DEFAULT 0,
			max_iterations INTEGER NOT NULL DEFAULT 10,
			current_gate TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			plan_json TEXT NOT NULL DEFAULT '',
			report TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0
		);`,
	)

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("db: migrate stmt failed: %w", err)
		}
	}

	// Additive column migrations (safe at user_version==schemaVersion).
	if err := ensureTaskRunOptionsColumns(ctx, tx); err != nil {
		return err
	}
	if err := ensureTasksColumns(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: migrate commit: %w", err)
	}
	return nil
}

func ensureTasksColumns(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("db: ensure tasks columns: tx is nil")
	}

	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(tasks);")
	if err != nil {
		return fmt.Errorf("db: ensure tasks columns: table_info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]bool{}
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
			return fmt.Errorf("db: ensure tasks columns: scan: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: ensure tasks columns: rows: %w", err)
	}

	type col struct {
		Name string
		Def  string
	}
	for _, c := range []col{
		{Name: "conversation_id", Def: "conversation_id TEXT NOT NULL DEFAULT ''"},
	} {
		if existing[c.Name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE tasks ADD COLUMN "+c.Def+";"); err != nil {
			return fmt.Errorf("db: ensure tasks columns: add %s: %w", c.Name, err)
		}
	}
	return nil
}

func ensureTaskRunOptionsColumns(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("db: ensure task_run_options columns: tx is nil")
	}

	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(task_run_options);")
	if err != nil {
		return fmt.Errorf("db: ensure task_run_options columns: table_info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	existing := map[string]bool{}
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
			return fmt.Errorf("db: ensure task_run_options columns: scan: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: ensure task_run_options columns: rows: %w", err)
	}

	type col struct {
		Name string
		Def  string
	}
	for _, c := range []col{
		{Name: "safety_preset", Def: "safety_preset TEXT NOT NULL DEFAULT ''"},
		{Name: "task_intent", Def: "task_intent TEXT NOT NULL DEFAULT ''"},
		{Name: "codex_sandbox", Def: "codex_sandbox TEXT NOT NULL DEFAULT ''"},
		{Name: "codex_approval_policy", Def: "codex_approval_policy TEXT NOT NULL DEFAULT ''"},
		{Name: "codex_search", Def: "codex_search INTEGER NOT NULL DEFAULT 0"},
		{Name: "claude_permission_mode", Def: "claude_permission_mode TEXT NOT NULL DEFAULT ''"},
		{Name: "claude_sandbox", Def: "claude_sandbox INTEGER NOT NULL DEFAULT 0"},
		{Name: "claude_webfetch_domains_json", Def: "claude_webfetch_domains_json TEXT NOT NULL DEFAULT ''"},
	} {
		if existing[c.Name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE task_run_options ADD COLUMN "+c.Def+";"); err != nil {
			return fmt.Errorf("db: ensure task_run_options columns: add %s: %w", c.Name, err)
		}
	}
	return nil
}
