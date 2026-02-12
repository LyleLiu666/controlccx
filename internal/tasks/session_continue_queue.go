package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) EnqueueSessionContinue(ctx context.Context, in EnqueueSessionContinueInput) (SessionContinueQueueItem, error) {
	if s == nil || s.db == nil {
		return SessionContinueQueueItem{}, errors.New("tasks: store not initialized")
	}
	conversationID := strings.TrimSpace(in.ConversationID)
	if conversationID == "" {
		return SessionContinueQueueItem{}, errors.New("tasks: conversation_id is required")
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return SessionContinueQueueItem{}, errors.New("tasks: prompt is required")
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "continue"
	}
	now := toMillis(s.now().UTC())
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_continue_queue (
			id, conversation_id, prompt, run_options_json, priority, state, source, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, id, conversationID, prompt, strings.TrimSpace(in.RunOptionsJSON), in.Priority, string(SessionContinueQueueStatePending), source, now, now)
	if err != nil {
		return SessionContinueQueueItem{}, fmt.Errorf("tasks: enqueue session continue: %w", err)
	}
	return s.GetSessionContinueQueueItem(ctx, id)
}

func (s *Store) GetSessionContinueQueueItem(ctx context.Context, id string) (SessionContinueQueueItem, error) {
	if s == nil || s.db == nil {
		return SessionContinueQueueItem{}, errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionContinueQueueItem{}, errors.New("tasks: queue id is required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, conversation_id, prompt, run_options_json, priority, state, source, created_at, updated_at
		FROM session_continue_queue
		WHERE id = ?;
	`, id)
	item, err := scanSessionContinueQueueItem(row)
	if err != nil {
		return SessionContinueQueueItem{}, err
	}
	return item, nil
}

func (s *Store) ListSessionContinueQueueByConversation(ctx context.Context, conversationID string, limit int) ([]SessionContinueQueueItem, error) {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, prompt, run_options_json, priority, state, source, created_at, updated_at
		FROM session_continue_queue
		WHERE conversation_id = ?
			AND state IN (?, ?)
		ORDER BY
			CASE WHEN state = ? THEN 0 ELSE 1 END,
			priority DESC,
			created_at ASC,
			id ASC
		LIMIT ?;
	`, conversationID, string(SessionContinueQueueStatePending), string(SessionContinueQueueStateDispatching), string(SessionContinueQueueStateDispatching), limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list session continue queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionContinueQueueItem
	for rows.Next() {
		item, err := scanSessionContinueQueueItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list session continue queue rows: %w", err)
	}
	return out, nil
}

func (s *Store) ListSessionContinueQueueConversations(ctx context.Context, limit int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			q.conversation_id,
			COALESCE(NULLIF((
				SELECT COALESCE(NULLIF(o.base_workdir, ''), NULLIF(t.workdir, ''))
				FROM tasks t
				LEFT JOIN task_run_options o ON o.task_id = t.id
				WHERE t.conversation_id = q.conversation_id
				ORDER BY t.created_at DESC, t.id DESC
				LIMIT 1
			), ''), q.conversation_id) AS project_scope,
			MIN(q.created_at) AS first_created_at
		FROM session_continue_queue q
		WHERE q.state = ?
		GROUP BY q.conversation_id
		ORDER BY first_created_at ASC, q.conversation_id ASC;
	`, string(SessionContinueQueueStatePending))
	if err != nil {
		return nil, fmt.Errorf("tasks: list queue conversations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type convRow struct {
		ConversationID string
		ProjectScope   string
	}
	var ordered []convRow
	for rows.Next() {
		var r convRow
		var firstCreatedAt int64
		if err := rows.Scan(&r.ConversationID, &r.ProjectScope, &firstCreatedAt); err != nil {
			return nil, fmt.Errorf("tasks: scan queue conversation: %w", err)
		}
		r.ConversationID = strings.TrimSpace(r.ConversationID)
		r.ProjectScope = strings.TrimSpace(r.ProjectScope)
		if r.ConversationID == "" {
			continue
		}
		if r.ProjectScope == "" {
			r.ProjectScope = r.ConversationID
		}
		ordered = append(ordered, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list queue conversations rows: %w", err)
	}

	if len(ordered) == 0 {
		return nil, nil
	}

	// Isolate scheduling by project scope: one round picks at most one conversation
	// from each scope so a single project backlog cannot starve others entirely.
	scopeOrder := make([]string, 0, len(ordered))
	byScope := make(map[string][]string, len(ordered))
	for _, r := range ordered {
		scope := strings.TrimSpace(r.ProjectScope)
		if _, ok := byScope[scope]; !ok {
			scopeOrder = append(scopeOrder, scope)
		}
		byScope[scope] = append(byScope[scope], r.ConversationID)
	}

	out := make([]string, 0, len(ordered))
	for len(out) < limit {
		progressed := false
		for _, scope := range scopeOrder {
			q := byScope[scope]
			if len(q) == 0 {
				continue
			}
			out = append(out, q[0])
			byScope[scope] = q[1:]
			progressed = true
			if len(out) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return out, nil
}

func (s *Store) ClaimNextSessionContinue(ctx context.Context, conversationID string) (SessionContinueQueueItem, bool, error) {
	if s == nil || s.db == nil {
		return SessionContinueQueueItem{}, false, errors.New("tasks: store not initialized")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return SessionContinueQueueItem{}, false, errors.New("tasks: conversation_id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionContinueQueueItem{}, false, fmt.Errorf("tasks: begin claim session continue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM session_continue_queue
		WHERE conversation_id = ? AND state = ?
		ORDER BY priority DESC, created_at ASC, id ASC
		LIMIT 1;
	`, conversationID, string(SessionContinueQueueStatePending)).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionContinueQueueItem{}, false, nil
		}
		return SessionContinueQueueItem{}, false, fmt.Errorf("tasks: select session continue claim: %w", err)
	}
	now := toMillis(s.now().UTC())
	res, err := tx.ExecContext(ctx, `
		UPDATE session_continue_queue
		SET state = ?, updated_at = ?
		WHERE id = ? AND state = ?;
	`, string(SessionContinueQueueStateDispatching), now, strings.TrimSpace(id), string(SessionContinueQueueStatePending))
	if err != nil {
		return SessionContinueQueueItem{}, false, fmt.Errorf("tasks: update session continue claim: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SessionContinueQueueItem{}, false, nil
	}

	row := tx.QueryRowContext(ctx, `
		SELECT id, conversation_id, prompt, run_options_json, priority, state, source, created_at, updated_at
		FROM session_continue_queue
		WHERE id = ?;
	`, strings.TrimSpace(id))
	item, err := scanSessionContinueQueueItem(row)
	if err != nil {
		return SessionContinueQueueItem{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return SessionContinueQueueItem{}, false, fmt.Errorf("tasks: commit claim session continue: %w", err)
	}
	return item, true, nil
}

func (s *Store) MarkSessionContinueQueueState(ctx context.Context, id string, state SessionContinueQueueState) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tasks: queue id is required")
	}
	switch state {
	case SessionContinueQueueStatePending, SessionContinueQueueStateDispatching, SessionContinueQueueStateDone, SessionContinueQueueStateCanceled, SessionContinueQueueStateFailed:
		// ok
	default:
		return fmt.Errorf("tasks: invalid queue state %q", state)
	}
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE session_continue_queue
		SET state = ?, updated_at = ?
		WHERE id = ?;
	`, string(state), now, id)
	if err != nil {
		return fmt.Errorf("tasks: mark session continue queue state: %w", err)
	}
	return nil
}

func (s *Store) SessionContinueQueuePosition(ctx context.Context, id string) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("tasks: store not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, errors.New("tasks: queue id is required")
	}
	var (
		cid       string
		priority  int
		createdAt int64
		state     string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT conversation_id, priority, created_at, state
		FROM session_continue_queue
		WHERE id = ?;
	`, id).Scan(&cid, &priority, &createdAt, &state)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("tasks: not found")
		}
		return 0, fmt.Errorf("tasks: read queue position: %w", err)
	}
	if strings.TrimSpace(state) != string(SessionContinueQueueStatePending) && strings.TrimSpace(state) != string(SessionContinueQueueStateDispatching) {
		return 0, nil
	}

	var n int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM session_continue_queue
		WHERE conversation_id = ?
			AND state = ?
			AND (
				priority > ?
				OR (priority = ? AND created_at < ?)
				OR (priority = ? AND created_at = ? AND id <= ?)
			);
	`, strings.TrimSpace(cid), string(SessionContinueQueueStatePending), priority, priority, createdAt, priority, createdAt, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("tasks: count queue position: %w", err)
	}
	if n < 1 {
		n = 1
	}
	return n, nil
}

func (s *Store) ResetDispatchingSessionContinueToPending(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("tasks: store not initialized")
	}
	now := toMillis(s.now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE session_continue_queue
		SET state = ?, updated_at = ?
		WHERE state = ?;
	`, string(SessionContinueQueueStatePending), now, string(SessionContinueQueueStateDispatching))
	if err != nil {
		return 0, fmt.Errorf("tasks: reset dispatching queue state: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanSessionContinueQueueItem(row rowScanner) (SessionContinueQueueItem, error) {
	var (
		item              SessionContinueQueueItem
		state             string
		createdAt, update int64
	)
	err := row.Scan(
		&item.ID, &item.ConversationID, &item.Prompt, &item.RunOptionsJSON, &item.Priority,
		&state, &item.Source, &createdAt, &update,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionContinueQueueItem{}, fmt.Errorf("tasks: not found")
		}
		return SessionContinueQueueItem{}, fmt.Errorf("tasks: scan session continue queue: %w", err)
	}
	item.ID = strings.TrimSpace(item.ID)
	item.ConversationID = strings.TrimSpace(item.ConversationID)
	item.Prompt = strings.TrimSpace(item.Prompt)
	item.RunOptionsJSON = strings.TrimSpace(item.RunOptionsJSON)
	item.Source = strings.TrimSpace(item.Source)
	item.State = SessionContinueQueueState(strings.TrimSpace(state))
	item.CreatedAt = fromMillis(createdAt)
	item.UpdatedAt = fromMillis(update)
	return item, nil
}
