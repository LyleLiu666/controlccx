package tasks

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_EnsureConversationIDs_MigratesLegacyKeys(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	store.now = func() time.Time { return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) }

	baseDir := t.TempDir()
	nowMs := toMillis(store.now().UTC())

	// Legacy: session-scoped metadata keyed by s:<session_id>, tasks missing conversation_id.
	const (
		taskIDLegacy = "task-legacy"
		sessionID    = "sess-legacy"
	)
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO tasks (
			id, worker_type, mode, status, prompt, workdir,
			session_id, conversation_id,
			created_at, updated_at, stderr_count, keyword_count, score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0);
	`, taskIDLegacy, string(WorkerClaudeCode), string(ModeNew), string(StatusSucceeded), "do stuff", baseDir,
		sessionID, "",
		nowMs, nowMs,
	); err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO session_meta (key, title, updated_at) VALUES (?, ?, ?);
	`, SessionKey("", sessionID), "Legacy Session", nowMs); err != nil {
		t.Fatalf("insert session_meta: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO acceptance_states (
			session_key, status, iteration, max_iterations, current_gate, summary, plan_json, report, run_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, SessionKey("", sessionID), "running", 1, 10, "gate", "sum", "{}", "rep", taskIDLegacy, nowMs); err != nil {
		t.Fatalf("insert acceptance_states: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO session_workspaces (
			key, kind, base_workdir, run_root, run_workdir, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`, SessionKey("", sessionID), "copy", baseDir, filepath.Join(baseDir, ".ccx", "workspaces", "legacy"), filepath.Join(baseDir, ".ccx", "workspaces", "legacy"), "active", nowMs, nowMs); err != nil {
		t.Fatalf("insert session_workspaces: %v", err)
	}

	// Legacy: task-scoped metadata keyed by t:<task_id>, tasks missing conversation_id.
	const taskIDNoSession = "task-nosession"
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO tasks (
			id, worker_type, mode, status, prompt, workdir,
			session_id, conversation_id,
			created_at, updated_at, stderr_count, keyword_count, score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0);
	`, taskIDNoSession, string(WorkerExec), string(ModeNew), string(StatusSucceeded), "echo hi", ".",
		"", "",
		nowMs, nowMs,
	); err != nil {
		t.Fatalf("insert no-session task: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO session_meta (key, title, updated_at) VALUES (?, ?, ?);
	`, SessionKey(taskIDNoSession, ""), "No Session Title", nowMs); err != nil {
		t.Fatalf("insert session_meta (t:key): %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO session_workspaces (
			key, kind, base_workdir, run_root, run_workdir, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`, SessionKey(taskIDNoSession, ""), "copy", baseDir, filepath.Join(baseDir, ".ccx", "workspaces", "nosession"), filepath.Join(baseDir, ".ccx", "workspaces", "nosession"), "active", nowMs, nowMs); err != nil {
		t.Fatalf("insert session_workspaces (t:key): %v", err)
	}

	if err := store.EnsureConversationIDs(ctx); err != nil {
		t.Fatalf("EnsureConversationIDs: %v", err)
	}

	legacy, err := store.GetTask(ctx, taskIDLegacy)
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if strings.TrimSpace(legacy.ConversationID) == "" {
		t.Fatalf("legacy conversation_id should be populated")
	}
	if legacy.SessionTitle != "Legacy Session" {
		t.Fatalf("legacy title=%q, want %q", legacy.SessionTitle, "Legacy Session")
	}
	legacyKey := SessionKeyForTask(legacy)
	if !strings.HasPrefix(legacyKey, "c:") {
		t.Fatalf("legacy session key=%q, want c:<conversation_id>", legacyKey)
	}

	// Acceptance mapping migrated to conversation key.
	if st, ok, err := store.GetAcceptanceState(ctx, legacyKey); err != nil || !ok {
		t.Fatalf("expected acceptance at conversation key; ok=%v err=%v", ok, err)
	} else if strings.TrimSpace(st.Status) != "running" {
		t.Fatalf("acceptance status=%q, want %q", st.Status, "running")
	}

	// Legacy keys are gone.
	if _, ok, _ := store.GetAcceptanceState(ctx, SessionKey("", sessionID)); ok {
		t.Fatalf("expected legacy acceptance key removed")
	}
	if err := conn.QueryRowContext(ctx, `SELECT key FROM session_workspaces WHERE key = ?;`, legacyKey).Scan(new(string)); err != nil {
		t.Fatalf("expected session_workspaces at conversation key: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT key FROM session_workspaces WHERE key = ?;`, SessionKey("", sessionID)).Scan(new(string)); err == nil {
		t.Fatalf("expected legacy session_workspaces key removed")
	}

	noSession, err := store.GetTask(ctx, taskIDNoSession)
	if err != nil {
		t.Fatalf("get no-session: %v", err)
	}
	if strings.TrimSpace(noSession.ConversationID) != taskIDNoSession {
		t.Fatalf("no-session conversation_id=%q, want task id %q", noSession.ConversationID, taskIDNoSession)
	}
	if noSession.SessionTitle != "No Session Title" {
		t.Fatalf("no-session title=%q, want %q", noSession.SessionTitle, "No Session Title")
	}
	if SessionKeyForTask(noSession) != "c:"+taskIDNoSession {
		t.Fatalf("no-session key=%q, want %q", SessionKeyForTask(noSession), "c:"+taskIDNoSession)
	}
	if err := conn.QueryRowContext(ctx, `SELECT key FROM session_workspaces WHERE key = ?;`, SessionKeyForTask(noSession)).Scan(new(string)); err != nil {
		t.Fatalf("expected session_workspaces at no-session conversation key: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT key FROM session_workspaces WHERE key = ?;`, SessionKey(taskIDNoSession, "")).Scan(new(string)); err == nil {
		t.Fatalf("expected legacy no-session session_workspaces key removed")
	}
}
