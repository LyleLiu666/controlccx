package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) TouchTask(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tasks: task id is required")
	}
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET updated_at = ?
		WHERE id = ?;
	`, now, id)
	if err != nil {
		return fmt.Errorf("tasks: touch task: %w", err)
	}
	return nil
}

func (s *Store) ListStaleInFlightTasks(ctx context.Context, before time.Time, limit int) ([]Task, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	beforeMs := toMillis(before.UTC())

	rows, err := s.db.QueryContext(ctx, `
			SELECT
				t.id, t.conversation_id, t.worker_type, t.mode, t.status,
				COALESCE(o.unsafe_automation, 0),
				COALESCE(o.safety_preset, ''), COALESCE(o.task_intent, ''),
				COALESCE(o.network_tier, ''),
				COALESCE(o.workdir_strategy, ''), COALESCE(o.base_workdir, ''), COALESCE(o.worktree_dir, ''), COALESCE(o.worktree_branch, ''),
				COALESCE(o.codex_sandbox, ''), COALESCE(o.codex_approval_policy, ''), COALESCE(o.codex_search, 0),
			COALESCE(o.claude_permission_mode, ''), COALESCE(o.claude_sandbox, 0), COALESCE(o.claude_webfetch_domains_json, ''),
			t.prompt, t.workdir, t.session_id, COALESCE(sm.title, ''), sm.deleted_at,
			t.warning, t.error, t.exit_code,
			t.stderr_count, t.keyword_count, t.score,
			t.created_at, t.updated_at, t.started_at, t.finished_at
		FROM tasks t
		LEFT JOIN task_run_options o ON o.task_id = t.id
		LEFT JOIN session_meta sm ON sm.key = (
			CASE
				WHEN t.conversation_id IS NOT NULL AND t.conversation_id != '' THEN 'c:' || t.conversation_id
				WHEN t.session_id IS NOT NULL AND t.session_id != '' THEN 's:' || t.session_id
				ELSE 't:' || t.id
			END
		)
		WHERE t.status IN (?, ?, ?)
			AND t.updated_at <= ?
			AND sm.deleted_at IS NULL
		ORDER BY t.updated_at ASC, t.created_at ASC
		LIMIT ?;
	`, string(StatusQueued), string(StatusRunning), string(StatusAwaitingApproval), beforeMs, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list stale inflight: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Task, 0, limit)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		PopulateHints(&t)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: stale inflight rows: %w", err)
	}
	return out, nil
}

// InterruptTaskIfStaleInFlight marks a queued/running/awaiting_approval task as interrupted,
// only if it is still in-flight and its updated_at is older than (or equal to) staleBefore.
// Returns ok=false when the task is no longer stale/in-flight (or already finished).
func (s *Store) InterruptTaskIfStaleInFlight(
	ctx context.Context,
	id string,
	staleBefore time.Time,
	finishedAt time.Time,
	reason string,
) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, errors.New("tasks: task id is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "stale watchdog timeout"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("tasks: begin interrupt stale: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		stderrCount  int
		keywordCount int
		exitCode     *int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT stderr_count, keyword_count, exit_code
		FROM tasks
		WHERE id = ?;
	`, id).Scan(&stderrCount, &keywordCount, &exitCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("tasks: not found")
		}
		return false, fmt.Errorf("tasks: read for interrupt stale: %w", err)
	}

	score := ComputeScore(StatusInterrupted, stderrCount, keywordCount, exitCode)
	now := finishedAt.UTC()
	nowMs := toMillis(now)
	staleBeforeMs := toMillis(staleBefore.UTC())

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, error = ?, score = ?, finished_at = ?, updated_at = ?
		WHERE id = ?
			AND finished_at IS NULL
			AND status IN (?, ?, ?)
			AND updated_at <= ?;
	`,
		string(StatusInterrupted),
		reason,
		score,
		nowMs,
		nowMs,
		id,
		string(StatusQueued),
		string(StatusRunning),
		string(StatusAwaitingApproval),
		staleBeforeMs,
	)
	if err != nil {
		return false, fmt.Errorf("tasks: interrupt stale update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("tasks: commit interrupt stale: %w", err)
	}
	return true, nil
}
