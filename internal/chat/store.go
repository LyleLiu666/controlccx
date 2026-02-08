package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

func (s *Store) Append(ctx context.Context, role Role, content string) (Message, error) {
	ts := s.now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (ts, role, content)
		VALUES (?, ?, ?);
	`, toMillis(ts), string(role), content)
	if err != nil {
		return Message{}, fmt.Errorf("chat: append: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Message{}, fmt.Errorf("chat: id: %w", err)
	}
	return Message{ID: id, Time: ts, Role: role, Content: content}, nil
}

func (s *Store) List(ctx context.Context, afterID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, role, content
		FROM chat_messages
		WHERE id > ?
		ORDER BY id ASC
		LIMIT ?;
	`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("chat: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Message
	for rows.Next() {
		var (
			m        Message
			tsMillis int64
			roleStr  string
		)
		if err := rows.Scan(&m.ID, &tsMillis, &roleStr, &m.Content); err != nil {
			return nil, fmt.Errorf("chat: scan: %w", err)
		}
		m.Time = fromMillis(tsMillis)
		m.Role = Role(roleStr)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat: rows: %w", err)
	}
	return out, nil
}

// Tail returns the most recent messages in chronological order (oldest first).
func (s *Store) Tail(ctx context.Context, limit int) ([]Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, role, content
		FROM chat_messages
		ORDER BY id DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("chat: tail: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Message
	for rows.Next() {
		var (
			m        Message
			tsMillis int64
			roleStr  string
		)
		if err := rows.Scan(&m.ID, &tsMillis, &roleStr, &m.Content); err != nil {
			return nil, fmt.Errorf("chat: scan: %w", err)
		}
		m.Time = fromMillis(tsMillis)
		m.Role = Role(roleStr)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat: rows: %w", err)
	}

	// Reverse so callers see chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) Clear(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("chat: store not initialized")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chat_messages;`); err != nil {
		return fmt.Errorf("chat: clear: %w", err)
	}
	return nil
}

// PruneKeepLast keeps the most recent N messages (by autoincrement id) and deletes older entries.
func (s *Store) PruneKeepLast(ctx context.Context, keep int) error {
	if s == nil || s.db == nil {
		return errors.New("chat: store not initialized")
	}
	if keep <= 0 {
		return s.Clear(ctx)
	}
	var cutoff int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM chat_messages
		ORDER BY id DESC
		LIMIT 1 OFFSET ?;
	`, keep-1).Scan(&cutoff)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("chat: prune cutoff: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE id < ?;`, cutoff); err != nil {
		return fmt.Errorf("chat: prune: %w", err)
	}
	return nil
}

func toMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}

func fromMillis(ms int64) time.Time {
	return time.Unix(0, ms*int64(time.Millisecond)).UTC()
}
