package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
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

	now := s.now().UTC()
	id := uuid.NewString()

	createdAt := toMillis(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: begin create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id, worker_type, mode, status, prompt, workdir, session_id, warning,
			created_at, updated_at, stderr_count, keyword_count, score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0);
	`, id, string(in.WorkerType), string(in.Mode), string(StatusQueued), in.Prompt, workdir, in.SessionID, in.Warning, createdAt, createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: insert: %w", err)
	}

	unsafeInt := 0
	if in.UnsafeAutomation {
		unsafeInt = 1
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO task_run_options (task_id, unsafe_automation)
		VALUES (?, ?);
	`, id, unsafeInt)
	if err != nil {
		return Task{}, fmt.Errorf("tasks: insert run options: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("tasks: commit create: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			t.id, t.worker_type, t.mode, t.status, COALESCE(o.unsafe_automation, 0),
			t.prompt, t.workdir, t.session_id, COALESCE(sm.title, ''), sm.deleted_at,
			t.warning, t.error, t.exit_code,
			stderr_count, keyword_count, score,
			t.created_at, t.updated_at, t.started_at, t.finished_at
		FROM tasks t
		LEFT JOIN task_run_options o ON o.task_id = t.id
		LEFT JOIN session_meta sm ON sm.key = (
			CASE
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

func (s *Store) ListTasksWithOptions(ctx context.Context, limit int, opts ListTasksOptions) ([]Task, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	includeDeleted := 0
	if opts.IncludeDeleted {
		includeDeleted = 1
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			t.id, t.worker_type, t.mode, t.status, COALESCE(o.unsafe_automation, 0),
			t.prompt, t.workdir, t.session_id, COALESCE(sm.title, ''), sm.deleted_at,
			t.warning, t.error, t.exit_code,
			t.stderr_count, t.keyword_count, t.score,
			t.created_at, t.updated_at, t.started_at, t.finished_at
		FROM tasks t
		LEFT JOIN task_run_options o ON o.task_id = t.id
		LEFT JOIN session_meta sm ON sm.key = (
			CASE
				WHEN t.session_id IS NOT NULL AND t.session_id != '' THEN 's:' || t.session_id
				ELSE 't:' || t.id
			END
		)
		WHERE (? = 1 OR sm.deleted_at IS NULL)
		ORDER BY t.created_at DESC
		LIMIT ?;
	`, includeDeleted, limit)
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

func (s *Store) MarkInterrupted(ctx context.Context) (int64, error) {
	now := toMillis(s.now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = ?
		WHERE status IN (?, ?);
	`, string(StatusInterrupted), now, string(StatusRunning), string(StatusQueued))
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

func (s *Store) SetBlocked(ctx context.Context, id string) error {
	return s.setStatus(ctx, id, StatusBlocked)
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
		stderrCount  int
		keywordCount int
		prevSessionID string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT stderr_count, keyword_count, session_id
		FROM tasks
		WHERE id = ?;
	`, id).Scan(&stderrCount, &keywordCount, &prevSessionID)
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

	if strings.TrimSpace(prevSessionID) == "" && strings.TrimSpace(in.SessionID) != "" {
		if err := migrateSessionMetaKeyTx(tx, SessionKey(id, ""), SessionKey(id, in.SessionID), toMillis(now)); err != nil {
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
	if err := tx.QueryRowContext(ctx, `SELECT session_id FROM tasks WHERE id = ?;`, id).Scan(&prev); err != nil {
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

	if err := migrateSessionMetaKeyTx(tx, SessionKey(id, ""), SessionKey(id, sessionID), toMillis(now)); err != nil {
		return err
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

func (s *Store) ListLogs(ctx context.Context, taskID string, afterID int64, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, ts, stream, message
		FROM logs
		WHERE task_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?;
	`, taskID, afterID, limit)
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
		sessionTitle              string
		sessionDeletedAt          sql.NullInt64
		createdAt, updatedAt      int64
		startedAt, finishedAt     sql.NullInt64
		exitCode                  sql.NullInt64
	)
	err := row.Scan(
		&t.ID, &workerType, &mode, &status, &unsafeAutomation,
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
	t.UnsafeAutomation = unsafeAutomation != 0
	t.SessionTitle = strings.TrimSpace(sessionTitle)
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

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func fromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}
