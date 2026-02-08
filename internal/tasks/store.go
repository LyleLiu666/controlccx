package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db  *sql.DB
	now func() time.Time

	sessionWorkspacesHasWorkspaceIDMu    sync.Mutex
	sessionWorkspacesHasWorkspaceIDKnown bool
	sessionWorkspacesHasWorkspaceIDValue bool
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:  db,
		now: time.Now,
	}
}

func (s *Store) CreateTask(ctx context.Context, in CreateTaskInput) (Task, error) {
	if s.db == nil {
		return Task{}, errors.New("tasks: store not initialized")
	}
	if in.WorkerType == "" {
		return Task{}, errors.New("tasks: worker_type is required")
	}
	if in.Mode == "" {
		in.Mode = ModeNew
	}
	switch in.Mode {
	case ModeNew:
		// ok
	case ModeResume:
		if in.SessionID == "" {
			return Task{}, errors.New("tasks: session_id is required for resume mode")
		}
	default:
		return Task{}, fmt.Errorf("tasks: invalid mode %q", in.Mode)
	}
	if in.Prompt == "" {
		return Task{}, errors.New("tasks: prompt is required")
	}
	if in.WorkDir == "" {
		in.WorkDir = "."
	}
	workdir := filepath.Clean(in.WorkDir)
	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	workdirStrategy := strings.ToLower(strings.TrimSpace(in.WorkDirStrategy))
	switch workdirStrategy {
	case "", "exclusive":
		// ok
	case "wait", "worktree":
		// ok
	default:
		return Task{}, fmt.Errorf("tasks: invalid workdir_strategy %q", strings.TrimSpace(in.WorkDirStrategy))
	}

	now := s.now().UTC()
	id := uuid.NewString()
	conversationID := strings.TrimSpace(in.ConversationID)

	createdAt := toMillis(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: begin create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Fast-path idempotent replays: return the existing task before applying workdir locks.
	if idempotencyKey != "" {
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM tasks
			WHERE idempotency_key = ?
			LIMIT 1;
		`, idempotencyKey).Scan(&existingID)
		if err == nil {
			if err := tx.Commit(); err != nil {
				return Task{}, fmt.Errorf("tasks: commit create: %w", err)
			}
			return s.GetTask(ctx, existingID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Task{}, fmt.Errorf("tasks: get by idempotency_key: %w", err)
		}
	}

	initialStatus := StatusQueued

	// Mutual exclusion: disallow concurrent queued/running/waiting tasks per workdir.
	{
		var existingID, existingStatus string
		err := tx.QueryRowContext(ctx, `
			SELECT id, status
			FROM tasks
			WHERE workdir = ?
				AND status IN (?, ?, ?, ?)
			ORDER BY created_at DESC
			LIMIT 1;
		`, workdir, string(StatusQueued), string(StatusWaiting), string(StatusRunning), string(StatusAwaitingApproval)).Scan(&existingID, &existingStatus)
		if err == nil {
			switch workdirStrategy {
			case "wait":
				initialStatus = StatusWaiting
			default:
				return Task{}, &WorkDirBusyError{
					WorkDir:         workdir,
					ExistingWorkDir: workdir,
					ExistingTaskID:  existingID,
					ExistingStatus:  Status(existingStatus),
				}
			}
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Task{}, fmt.Errorf("tasks: check workdir busy: %w", err)
		}
	}

	if conversationID == "" {
		if in.Mode == ModeResume && strings.TrimSpace(in.SessionID) != "" {
			if err := tx.QueryRowContext(ctx, `
				SELECT conversation_id
				FROM tasks
				WHERE session_id = ? AND conversation_id IS NOT NULL AND conversation_id != ''
				ORDER BY created_at DESC
				LIMIT 1;
			`, strings.TrimSpace(in.SessionID)).Scan(&conversationID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return Task{}, fmt.Errorf("tasks: resolve conversation_id for session_id: %w", err)
			}
		}
		if conversationID == "" {
			conversationID = uuid.NewString()
		}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO tasks (
			id, worker_type, mode, status, prompt, workdir, session_id, conversation_id, idempotency_key, warning,
			created_at, updated_at, stderr_count, keyword_count, score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0);
	`, id, string(in.WorkerType), string(in.Mode), string(initialStatus), in.Prompt, workdir, in.SessionID, conversationID, idempotencyKey, in.Warning, createdAt, createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: insert: %w", err)
	}
	if idempotencyKey != "" {
		if rows, _ := res.RowsAffected(); rows == 0 {
			var existingID string
			if err := tx.QueryRowContext(ctx, `
				SELECT id
				FROM tasks
				WHERE idempotency_key = ?
				LIMIT 1;
			`, idempotencyKey).Scan(&existingID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return Task{}, fmt.Errorf("tasks: idempotency key conflict but existing row not found")
				}
				return Task{}, fmt.Errorf("tasks: get by idempotency_key: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return Task{}, fmt.Errorf("tasks: commit create: %w", err)
			}
			return s.GetTask(ctx, existingID)
		}
	}

	unsafeInt := 0
	if in.UnsafeAutomation {
		unsafeInt = 1
	}

	safetyPreset := strings.TrimSpace(in.SafetyPreset)
	taskIntent := strings.TrimSpace(in.TaskIntent)
	persistedWorkdirStrategy := workdirStrategy
	if persistedWorkdirStrategy == "" || persistedWorkdirStrategy == "exclusive" {
		persistedWorkdirStrategy = ""
	}
	baseWorkDir := strings.TrimSpace(in.BaseWorkDir)
	worktreeDir := strings.TrimSpace(in.WorktreeDir)
	worktreeBranch := strings.TrimSpace(in.WorktreeBranch)
	if workdirStrategy != "worktree" {
		baseWorkDir = ""
		worktreeDir = ""
		worktreeBranch = ""
	} else {
		baseWorkDir = filepath.Clean(baseWorkDir)
		worktreeDir = filepath.Clean(worktreeDir)
		if strings.TrimSpace(baseWorkDir) == "" || strings.TrimSpace(worktreeDir) == "" || worktreeBranch == "" {
			return Task{}, errors.New("tasks: worktree strategy requires base_workdir, worktree_dir, and worktree_branch")
		}
	}
	codexSandbox := strings.TrimSpace(in.CodexSandbox)
	codexApproval := strings.TrimSpace(in.CodexApprovalPolicy)
	codexSearch := 0
	if in.CodexSearch {
		codexSearch = 1
	}
	claudePermissionMode := strings.TrimSpace(in.ClaudePermissionMode)
	claudeSandbox := 0
	if in.ClaudeSandbox {
		claudeSandbox = 1
	}
	claudeDomains := normalizeDomains(in.ClaudeWebFetchDomains)
	claudeDomainsJSON := ""
	if len(claudeDomains) > 0 {
		if b, err := json.Marshal(claudeDomains); err == nil {
			claudeDomainsJSON = string(b)
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO task_run_options (
			task_id, unsafe_automation,
			safety_preset, task_intent,
			workdir_strategy, base_workdir, worktree_dir, worktree_branch,
			codex_sandbox, codex_approval_policy, codex_search,
			claude_permission_mode, claude_sandbox, claude_webfetch_domains_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, id, unsafeInt,
		safetyPreset, taskIntent,
		persistedWorkdirStrategy, baseWorkDir, worktreeDir, worktreeBranch,
		codexSandbox, codexApproval, codexSearch,
		claudePermissionMode, claudeSandbox, claudeDomainsJSON,
	)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: insert run options: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("tasks: commit create: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) GetTaskByIdempotencyKey(ctx context.Context, key string) (Task, bool, error) {
	if s == nil || s.db == nil {
		return Task{}, false, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return Task{}, false, nil
	}

	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM tasks
		WHERE idempotency_key = ?
		LIMIT 1;
	`, key).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, false, nil
		}
		return Task{}, false, fmt.Errorf("tasks: get by idempotency_key: %w", err)
	}
	t, err := s.GetTask(ctx, strings.TrimSpace(id))
	if err != nil {
		return Task{}, false, err
	}
	return t, true, nil
}

func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			t.id, t.conversation_id, t.worker_type, t.mode, t.status,
			COALESCE(o.unsafe_automation, 0),
			COALESCE(o.safety_preset, ''), COALESCE(o.task_intent, ''),
			COALESCE(o.workdir_strategy, ''), COALESCE(o.base_workdir, ''), COALESCE(o.worktree_dir, ''), COALESCE(o.worktree_branch, ''),
			COALESCE(o.codex_sandbox, ''), COALESCE(o.codex_approval_policy, ''), COALESCE(o.codex_search, 0),
			COALESCE(o.claude_permission_mode, ''), COALESCE(o.claude_sandbox, 0), COALESCE(o.claude_webfetch_domains_json, ''),
			t.prompt, t.workdir, t.session_id, COALESCE(sm.title, ''), sm.deleted_at,
			t.warning, t.error, t.exit_code,
			stderr_count, keyword_count, score,
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
		WHERE t.id = ?;
	`, id)
	task, err := scanTask(row)
	if err != nil {
		return Task{}, err
	}
	PopulateHints(&task)
	return task, nil
}

func (s *Store) ListTasks(ctx context.Context, limit int) ([]Task, error) {
	return s.ListTasksWithOptions(ctx, limit, ListTasksOptions{})
}

func (s *Store) CountByStatus(ctx context.Context, opts ListTasksOptions) (map[Status]int, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, errors.New("tasks: store not initialized")
	}
	query := `
		SELECT t.status, COUNT(*)
		FROM tasks t
		LEFT JOIN session_meta sm ON sm.key = (
			CASE
				WHEN t.conversation_id IS NOT NULL AND t.conversation_id != '' THEN 'c:' || t.conversation_id
				WHEN t.session_id IS NOT NULL AND t.session_id != '' THEN 's:' || t.session_id
				ELSE 't:' || t.id
			END
		)
	`
	if !opts.IncludeDeleted {
		query += `
		WHERE sm.deleted_at IS NULL
	`
	}
	query += `
		GROUP BY t.status;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("tasks: count by status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[Status]int{}
	total := 0
	for rows.Next() {
		var (
			status string
			n      int64
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, 0, fmt.Errorf("tasks: count by status scan: %w", err)
		}
		st := Status(strings.TrimSpace(status))
		out[st] = int(n)
		total += int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("tasks: count by status rows: %w", err)
	}
	return out, total, nil
}

func (s *Store) ListTasksWithOptions(ctx context.Context, limit int, opts ListTasksOptions) ([]Task, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT
			t.id, t.conversation_id, t.worker_type, t.mode, t.status,
			COALESCE(o.unsafe_automation, 0),
			COALESCE(o.safety_preset, ''), COALESCE(o.task_intent, ''),
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
	`
	args := make([]any, 0, 1)
	if !opts.IncludeDeleted {
		query += `
		WHERE sm.deleted_at IS NULL
	`
	}
	query += `
		ORDER BY
			COALESCE(t.finished_at, t.started_at, t.created_at) DESC,
			t.created_at DESC,
			t.id DESC
		LIMIT ?;
	`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		PopulateHints(&task)
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list rows: %w", err)
	}
	return out, nil
}

func (s *Store) ConversationIDForSessionID(ctx context.Context, sessionID string) (string, bool, error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("tasks: store not initialized")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", false, nil
	}

	var cid string
	err := s.db.QueryRowContext(ctx, `
		SELECT conversation_id
		FROM tasks
		WHERE session_id = ? AND conversation_id IS NOT NULL AND conversation_id != ''
		ORDER BY created_at DESC
		LIMIT 1;
	`, sessionID).Scan(&cid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("tasks: resolve conversation_id for session_id: %w", err)
	}
	cid = strings.TrimSpace(cid)
	if cid == "" {
		return "", false, nil
	}
	return cid, true, nil
}

func (s *Store) ListTasksByConversationID(ctx context.Context, conversationID string, limit int, opts ListTasksOptions) ([]Task, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("tasks: conversation_id is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT
			t.id, t.conversation_id, t.worker_type, t.mode, t.status,
			COALESCE(o.unsafe_automation, 0),
			COALESCE(o.safety_preset, ''), COALESCE(o.task_intent, ''),
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
		WHERE t.conversation_id = ?
	`
	args := []any{conversationID}
	if !opts.IncludeDeleted {
		query += `
			AND sm.deleted_at IS NULL
	`
	}
	query += `
		ORDER BY t.created_at DESC
		LIMIT ?;
	`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list by conversation_id: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		PopulateHints(&task)
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list by conversation_id rows: %w", err)
	}
	return out, nil
}

func (s *Store) MarkInterrupted(ctx context.Context) (int64, error) {
	now := toMillis(s.now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = ?
		WHERE status IN (?, ?, ?);
	`, string(StatusInterrupted), now, string(StatusRunning), string(StatusQueued), string(StatusAwaitingApproval))
	if err != nil {
		return 0, fmt.Errorf("tasks: mark interrupted: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) SetRunning(ctx context.Context, id string) error {
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, started_at = ?, updated_at = ?
		WHERE id = ?;
	`, string(StatusRunning), now, now, id)
	if err != nil {
		return fmt.Errorf("tasks: set running: %w", err)
	}
	return nil
}

// DequeueNextWaitingForWorkdir moves the oldest waiting task for workdir into queued,
// as long as no other queued/running task exists for that workdir.
func (s *Store) DequeueNextWaitingForWorkdir(ctx context.Context, workdir string) (Task, bool, error) {
	if s == nil || s.db == nil {
		return Task{}, false, errors.New("tasks: store not initialized")
	}
	if workdir == "" {
		workdir = "."
	}
	workdir = filepath.Clean(workdir)

	now := s.now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, false, fmt.Errorf("tasks: begin dequeue waiting: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Only one active task per workdir: if something is queued/running, keep waiting.
	{
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM tasks
			WHERE workdir = ? AND status IN (?, ?, ?)
			ORDER BY created_at DESC
			LIMIT 1;
		`, workdir, string(StatusQueued), string(StatusRunning), string(StatusAwaitingApproval)).Scan(&existingID)
		if err == nil {
			return Task{}, false, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Task{}, false, fmt.Errorf("tasks: check active for dequeue: %w", err)
		}
	}

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM tasks
		WHERE workdir = ? AND status = ?
		ORDER BY created_at ASC
		LIMIT 1;
	`, workdir, string(StatusWaiting)).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, false, nil
		}
		return Task{}, false, fmt.Errorf("tasks: select waiting: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ?;
	`, string(StatusQueued), toMillis(now), strings.TrimSpace(id), string(StatusWaiting))
	if err != nil {
		return Task{}, false, fmt.Errorf("tasks: promote waiting: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return Task{}, false, nil
	}

	if err := tx.Commit(); err != nil {
		return Task{}, false, fmt.Errorf("tasks: commit dequeue waiting: %w", err)
	}
	next, err := s.GetTask(ctx, strings.TrimSpace(id))
	if err != nil {
		return Task{}, false, err
	}
	return next, true, nil
}

func (s *Store) SetBlocked(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusBlocked)
}

func (s *Store) SetAwaitingApproval(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusAwaitingApproval)
}

// SetRunningStatus updates the task status back to running without touching started_at.
// This is useful when a run temporarily pauses (e.g. awaiting approvals) and later resumes.
func (s *Store) SetRunningStatus(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusRunning)
}

func (s *Store) SetCanceled(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusCanceled)
}

func (s *Store) setStatus(ctx context.Context, id string, status Status) error {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks: begin: %w", err)
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
			return fmt.Errorf("tasks: not found")
		}
		return fmt.Errorf("tasks: read for status: %w", err)
	}

	score := ComputeScore(status, stderrCount, keywordCount, exitCode)
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, score = ?, updated_at = ?
		WHERE id = ?;
	`, string(status), score, toMillis(now), id)
	if err != nil {
		return fmt.Errorf("tasks: update status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tasks: commit status: %w", err)
	}
	return nil
}

func (s *Store) FinishTask(ctx context.Context, id string, in FinishTaskInput) error {
	now := in.FinishedAt.UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks: begin finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		stderrCount        int
		keywordCount       int
		prevSessionID      string
		prevConversationID string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT stderr_count, keyword_count, session_id, conversation_id
		FROM tasks
		WHERE id = ?;
	`, id).Scan(&stderrCount, &keywordCount, &prevSessionID, &prevConversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("tasks: not found")
		}
		return fmt.Errorf("tasks: read for finish: %w", err)
	}

	score := ComputeScore(in.Status, stderrCount, keywordCount, in.ExitCode)
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, session_id = COALESCE(NULLIF(?, ''), session_id),
			exit_code = ?, error = ?, score = ?, finished_at = ?, updated_at = ?
		WHERE id = ?;
	`, string(in.Status), in.SessionID, in.ExitCode, in.Error, score, toMillis(now), toMillis(now), id)
	if err != nil {
		return fmt.Errorf("tasks: finish update: %w", err)
	}

	// Legacy compatibility: if a task has no conversation_id, session metadata was historically keyed
	// by task id until session_id became known at finish time.
	if strings.TrimSpace(prevConversationID) == "" && strings.TrimSpace(prevSessionID) == "" && strings.TrimSpace(in.SessionID) != "" {
		nowMs := toMillis(now)
		if err := migrateSessionMetaKeyTx(tx, SessionKey(id, ""), SessionKey(id, in.SessionID), nowMs); err != nil {
			return err
		}
		if err := migrateSessionWorkspacesKeyTx(tx, SessionKey(id, ""), SessionKey(id, in.SessionID), nowMs); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tasks: finish commit: %w", err)
	}
	return nil
}

func (s *Store) SetSessionID(ctx context.Context, id, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	now := s.now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks: begin set session_id: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var prev string
	var prevConversationID string
	if err := tx.QueryRowContext(ctx, `SELECT session_id, conversation_id FROM tasks WHERE id = ?;`, id).Scan(&prev, &prevConversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("tasks: not found")
		}
		return fmt.Errorf("tasks: read session_id: %w", err)
	}
	if strings.TrimSpace(prev) != "" {
		return nil
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET session_id = ?, updated_at = ?
		WHERE id = ? AND (session_id IS NULL OR session_id = '');
	`, sessionID, toMillis(now), id)
	if err != nil {
		return fmt.Errorf("tasks: set session_id: %w", err)
	}

	nowMs := toMillis(now)
	if strings.TrimSpace(prevConversationID) == "" {
		if err := migrateSessionMetaKeyTx(tx, SessionKey(id, ""), SessionKey(id, sessionID), nowMs); err != nil {
			return err
		}
		if err := migrateSessionWorkspacesKeyTx(tx, SessionKey(id, ""), SessionKey(id, sessionID), nowMs); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tasks: commit set session_id: %w", err)
	}
	return nil
}

func (s *Store) SetWarning(ctx context.Context, id, warning string) error {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return nil
	}
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET warning = ?, updated_at = ?
		WHERE id = ?;
	`, warning, now, id)
	if err != nil {
		return fmt.Errorf("tasks: set warning: %w", err)
	}
	return nil
}

func (s *Store) AppendLog(ctx context.Context, taskID string, stream LogStream, message string) (LogEntry, error) {
	now := s.now().UTC()
	ts := toMillis(now)

	stderrInc := 0
	if stream == LogStderr {
		stderrInc = 1
	}
	keywordInc := CountKeywordHits(message)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LogEntry{}, fmt.Errorf("tasks: begin append log: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO logs (task_id, ts, stream, message)
		VALUES (?, ?, ?, ?);
	`, taskID, ts, string(stream), message)
	if err != nil {
		return LogEntry{}, fmt.Errorf("tasks: insert log: %w", err)
	}
	logID, err := res.LastInsertId()
	if err != nil {
		return LogEntry{}, fmt.Errorf("tasks: log id: %w", err)
	}

	var (
		status       Status
		stderrCount  int
		keywordCount int
		exitCode     *int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT status, stderr_count, keyword_count, exit_code
		FROM tasks
		WHERE id = ?;
	`, taskID).Scan(&status, &stderrCount, &keywordCount, &exitCode)
	if err != nil {
		return LogEntry{}, fmt.Errorf("tasks: read for log update: %w", err)
	}

	stderrCount += stderrInc
	keywordCount += keywordInc
	score := ComputeScore(status, stderrCount, keywordCount, exitCode)

	_, err = tx.ExecContext(ctx, `
		UPDATE tasks
		SET stderr_count = ?, keyword_count = ?, score = ?, updated_at = ?
		WHERE id = ?;
	`, stderrCount, keywordCount, score, ts, taskID)
	if err != nil {
		return LogEntry{}, fmt.Errorf("tasks: update counters: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return LogEntry{}, fmt.Errorf("tasks: commit append log: %w", err)
	}

	return LogEntry{
		ID:      logID,
		TaskID:  taskID,
		Time:    now,
		Stream:  stream,
		Message: message,
	}, nil
}

func (s *Store) SetInvocation(ctx context.Context, taskID string, cmd string, args []string, dir string, envKeys []string) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("tasks: task_id is required")
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return errors.New("tasks: cmd is required")
	}
	dir = filepath.Clean(strings.TrimSpace(dir))

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("tasks: marshal args: %w", err)
	}
	envJSON, err := json.Marshal(envKeys)
	if err != nil {
		return fmt.Errorf("tasks: marshal env keys: %w", err)
	}

	now := toMillis(s.now().UTC())
	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO task_invocations (task_id, cmd, args_json, dir, env_keys_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?);
	`, taskID, cmd, string(argsJSON), dir, string(envJSON), now)
	if err != nil {
		return fmt.Errorf("tasks: set invocation: %w", err)
	}
	return nil
}

func (s *Store) GetInvocation(ctx context.Context, taskID string) (Invocation, bool, error) {
	if s == nil || s.db == nil {
		return Invocation{}, false, errors.New("tasks: store not initialized")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Invocation{}, false, errors.New("tasks: task_id is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT task_id, cmd, args_json, dir, env_keys_json, created_at
		FROM task_invocations
		WHERE task_id = ?;
	`, taskID)

	var (
		out       Invocation
		argsRaw   string
		envRaw    string
		createdMs int64
	)
	if err := row.Scan(&out.TaskID, &out.Cmd, &argsRaw, &out.Dir, &envRaw, &createdMs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Invocation{}, false, nil
		}
		return Invocation{}, false, fmt.Errorf("tasks: get invocation: %w", err)
	}

	_ = json.Unmarshal([]byte(argsRaw), &out.Args)
	_ = json.Unmarshal([]byte(envRaw), &out.EnvInjectedKeys)
	out.CreatedAt = fromMillis(createdMs)
	return out, true, nil
}

type ListLogsFilter struct {
	Streams []LogStream
	Query   string
}

func (s *Store) ListLogs(ctx context.Context, taskID string, afterID int64, limit int) ([]LogEntry, error) {
	return s.ListLogsFiltered(ctx, taskID, afterID, limit, ListLogsFilter{})
}

func (s *Store) ListLogsFiltered(ctx context.Context, taskID string, afterID int64, limit int, filter ListLogsFilter) ([]LogEntry, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	return s.listLogsFiltered(ctx, taskID, afterID, limit, filter)
}

func (s *Store) ListAllLogsFiltered(ctx context.Context, taskID string, filter ListLogsFilter) ([]LogEntry, error) {
	// For exports: allow large responses (still bounded to avoid pathological memory use).
	return s.listLogsFiltered(ctx, taskID, 0, 200000, filter)
}

func (s *Store) listLogsFiltered(ctx context.Context, taskID string, afterID int64, limit int, filter ListLogsFilter) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 200000 {
		limit = 200000
	}
	query := strings.TrimSpace(filter.Query)
	var streams []string
	for _, s := range filter.Streams {
		if strings.TrimSpace(string(s)) == "" {
			continue
		}
		streams = append(streams, string(s))
	}

	var (
		sb   strings.Builder
		args []any
	)
	sb.WriteString(`
		SELECT id, task_id, ts, stream, message
		FROM logs
		WHERE task_id = ? AND id > ?
	`)
	args = append(args, taskID, afterID)

	if len(streams) > 0 {
		sb.WriteString(" AND stream IN (")
		for i, v := range streams {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("?")
			args = append(args, v)
		}
		sb.WriteString(")")
	}
	if query != "" {
		sb.WriteString(" AND instr(lower(message), lower(?)) > 0")
		args = append(args, query)
	}
	sb.WriteString(" ORDER BY id ASC LIMIT ?;")
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []LogEntry
	for rows.Next() {
		var (
			e         LogEntry
			tsMillis  int64
			streamStr string
		)
		if err := rows.Scan(&e.ID, &e.TaskID, &tsMillis, &streamStr, &e.Message); err != nil {
			return nil, fmt.Errorf("tasks: scan logs: %w", err)
		}
		e.Time = fromMillis(tsMillis)
		e.Stream = LogStream(streamStr)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list logs rows: %w", err)
	}
	return out, nil
}

func (s *Store) LatestLog(ctx context.Context, taskID string, stream LogStream) (LogEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, ts, stream, message
		FROM logs
		WHERE task_id = ? AND stream = ?
		ORDER BY id DESC
		LIMIT 1;
	`, taskID, string(stream))

	var (
		e         LogEntry
		tsMillis  int64
		streamStr string
	)
	if err := row.Scan(&e.ID, &e.TaskID, &tsMillis, &streamStr, &e.Message); err != nil {
		return LogEntry{}, fmt.Errorf("tasks: latest log: %w", err)
	}
	e.Time = fromMillis(tsMillis)
	e.Stream = LogStream(streamStr)
	return e, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (Task, error) {
	var (
		t                         Task
		workerType, mode, status  string
		unsafeAutomation          int
		safetyPreset              string
		taskIntent                string
		workdirStrategy           string
		baseWorkDir               string
		worktreeDir               string
		worktreeBranch            string
		codexSandbox              string
		codexApproval             string
		codexSearch               int
		claudePermissionMode      string
		claudeSandbox             int
		claudeWebFetchDomainsJSON string
		sessionTitle              string
		sessionDeletedAt          sql.NullInt64
		createdAt, updatedAt      int64
		startedAt, finishedAt     sql.NullInt64
		exitCode                  sql.NullInt64
	)
	err := row.Scan(
		&t.ID, &t.ConversationID, &workerType, &mode, &status,
		&unsafeAutomation,
		&safetyPreset, &taskIntent,
		&workdirStrategy, &baseWorkDir, &worktreeDir, &worktreeBranch,
		&codexSandbox, &codexApproval, &codexSearch,
		&claudePermissionMode, &claudeSandbox, &claudeWebFetchDomainsJSON,
		&t.Prompt, &t.WorkDir, &t.SessionID, &sessionTitle, &sessionDeletedAt,
		&t.Warning, &t.Error, &exitCode,
		&t.StderrCount, &t.KeywordCount, &t.Score,
		&createdAt, &updatedAt, &startedAt, &finishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, fmt.Errorf("tasks: not found")
		}
		return Task{}, fmt.Errorf("tasks: scan: %w", err)
	}

	t.WorkerType = WorkerType(workerType)
	t.Mode = Mode(mode)
	t.Status = Status(status)
	t.ConversationID = strings.TrimSpace(t.ConversationID)
	t.UnsafeAutomation = unsafeAutomation != 0
	t.WorkDirStrategy = strings.TrimSpace(workdirStrategy)
	t.SafetyPreset = strings.TrimSpace(safetyPreset)
	t.TaskIntent = strings.TrimSpace(taskIntent)
	t.CodexSandbox = strings.TrimSpace(codexSandbox)
	t.CodexApprovalPolicy = strings.TrimSpace(codexApproval)
	t.CodexSearch = codexSearch != 0
	t.ClaudePermissionMode = strings.TrimSpace(claudePermissionMode)
	t.ClaudeSandbox = claudeSandbox != 0
	if strings.TrimSpace(claudeWebFetchDomainsJSON) != "" {
		var domains []string
		if err := json.Unmarshal([]byte(claudeWebFetchDomainsJSON), &domains); err == nil {
			t.ClaudeWebFetchDomains = normalizeDomains(domains)
		}
	}
	t.SessionTitle = strings.TrimSpace(sessionTitle)
	t.BaseWorkDir = strings.TrimSpace(baseWorkDir)
	t.WorktreeDir = strings.TrimSpace(worktreeDir)
	t.WorktreeBranch = strings.TrimSpace(worktreeBranch)
	t.CreatedAt = fromMillis(createdAt)
	t.UpdatedAt = fromMillis(updatedAt)

	if sessionDeletedAt.Valid {
		v := fromMillis(sessionDeletedAt.Int64)
		t.SessionDeletedAt = &v
	}
	if startedAt.Valid {
		v := fromMillis(startedAt.Int64)
		t.StartedAt = &v
	}
	if finishedAt.Valid {
		v := fromMillis(finishedAt.Int64)
		t.FinishedAt = &v
	}
	if exitCode.Valid {
		v := int(exitCode.Int64)
		t.ExitCode = &v
	}
	return t, nil
}

func normalizeDomains(domains []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		key := strings.ToLower(d)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func fromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}
