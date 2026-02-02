package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type ListTasksOptions struct {
	IncludeDeleted bool
}

func SessionKey(taskID, sessionID string) string {
	sid := strings.TrimSpace(sessionID)
	if sid != "" {
		return "s:" + sid
	}
	return "t:" + strings.TrimSpace(taskID)
}

func ConversationKey(conversationID string) string {
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return ""
	}
	return "c:" + cid
}

func SessionKeyForTask(t Task) string {
	if cid := strings.TrimSpace(t.ConversationID); cid != "" {
		return ConversationKey(cid)
	}
	return SessionKey(t.ID, t.SessionID)
}

func (s *Store) RenameSession(ctx context.Context, key, title string) error {
	if s.db == nil {
		return fmt.Errorf("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	title = strings.TrimSpace(title)
	if key == "" {
		return fmt.Errorf("tasks: session key is required")
	}
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_meta (key, title, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			title = excluded.title,
			updated_at = excluded.updated_at;
	`, key, title, now)
	if err != nil {
		return fmt.Errorf("tasks: rename session: %w", err)
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, key string) error {
	if s.db == nil {
		return fmt.Errorf("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("tasks: session key is required")
	}
	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_meta (key, title, deleted_at, updated_at)
		VALUES (?, '', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			deleted_at = excluded.deleted_at,
			updated_at = excluded.updated_at;
	`, key, now, now)
	if err != nil {
		return fmt.Errorf("tasks: delete session: %w", err)
	}
	return nil
}

func migrateSessionMetaKeyTx(tx *sql.Tx, fromKey, toKey string, nowMs int64) error {
	fromKey = strings.TrimSpace(fromKey)
	toKey = strings.TrimSpace(toKey)
	if fromKey == "" || toKey == "" || fromKey == toKey {
		return nil
	}

	var (
		fromTitle   string
		fromDeleted sql.NullInt64
	)
	err := tx.QueryRow(`SELECT title, deleted_at FROM session_meta WHERE key = ?;`, fromKey).
		Scan(&fromTitle, &fromDeleted)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("tasks: read session_meta(from): %w", err)
	}

	var (
		toTitle   string
		toDeleted sql.NullInt64
	)
	err = tx.QueryRow(`SELECT title, deleted_at FROM session_meta WHERE key = ?;`, toKey).
		Scan(&toTitle, &toDeleted)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("tasks: read session_meta(to): %w", err)
	}

	if err == sql.ErrNoRows {
		_, err = tx.Exec(`UPDATE session_meta SET key = ?, updated_at = ? WHERE key = ?;`, toKey, nowMs, fromKey)
		if err != nil {
			return fmt.Errorf("tasks: migrate session_meta key: %w", err)
		}
		return nil
	}

	// Merge into toKey then remove fromKey.
	if strings.TrimSpace(toTitle) == "" && strings.TrimSpace(fromTitle) != "" {
		_, err = tx.Exec(`UPDATE session_meta SET title = ?, updated_at = ? WHERE key = ?;`, strings.TrimSpace(fromTitle), nowMs, toKey)
		if err != nil {
			return fmt.Errorf("tasks: merge session_meta title: %w", err)
		}
	}
	if !toDeleted.Valid && fromDeleted.Valid {
		_, err = tx.Exec(`UPDATE session_meta SET deleted_at = ?, updated_at = ? WHERE key = ?;`, fromDeleted.Int64, nowMs, toKey)
		if err != nil {
			return fmt.Errorf("tasks: merge session_meta deleted_at: %w", err)
		}
	}
	_, err = tx.Exec(`DELETE FROM session_meta WHERE key = ?;`, fromKey)
	if err != nil {
		return fmt.Errorf("tasks: delete session_meta(from): %w", err)
	}
	return nil
}
