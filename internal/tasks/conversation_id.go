package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EnsureConversationIDs backfills conversation_id for legacy tasks and migrates session-scoped state
// (session_meta, acceptance_states) to conversation-scoped keys.
//
// It is safe to call multiple times.
func (s *Store) EnsureConversationIDs(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks: begin EnsureConversationIDs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nowMs := toMillis(s.now().UTC())

	// Prefer the latest known conversation_id for each session_id (if present).
	existingBySession := map[string]string{}
	{
		rows, err := tx.QueryContext(ctx, `
			SELECT session_id, conversation_id
			FROM tasks
			WHERE session_id IS NOT NULL AND session_id != ''
				AND conversation_id IS NOT NULL AND conversation_id != ''
			ORDER BY created_at DESC;
		`)
		if err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: list existing mappings: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sid, cid string
			if err := rows.Scan(&sid, &cid); err != nil {
				return fmt.Errorf("tasks: EnsureConversationIDs: scan existing mapping: %w", err)
			}
			sid = strings.TrimSpace(sid)
			cid = strings.TrimSpace(cid)
			if sid == "" || cid == "" {
				continue
			}
			if _, ok := existingBySession[sid]; ok {
				continue
			}
			existingBySession[sid] = cid
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: existing mapping rows: %w", err)
		}
	}

	// Ensure legacy session-keyed rows are migrated even when tasks already have conversation_id.
	for sid, cid := range existingBySession {
		toKey := ConversationKey(cid)
		if toKey == "" {
			continue
		}
		fromKey := SessionKey("", sid)
		if err := migrateSessionMetaKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateSessionWorkspacesKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateAcceptanceStateKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
	}

	// Find tasks missing conversation_id.
	type legacyTask struct {
		ID        string
		SessionID string
	}
	bySession := map[string][]string{}
	var noSession []string
	{
		rows, err := tx.QueryContext(ctx, `
			SELECT id, session_id
			FROM tasks
			WHERE conversation_id IS NULL OR conversation_id = '';
		`)
		if err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: list legacy tasks: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var t legacyTask
			if err := rows.Scan(&t.ID, &t.SessionID); err != nil {
				return fmt.Errorf("tasks: EnsureConversationIDs: scan legacy task: %w", err)
			}
			t.ID = strings.TrimSpace(t.ID)
			t.SessionID = strings.TrimSpace(t.SessionID)
			if t.ID == "" {
				continue
			}
			if t.SessionID != "" {
				bySession[t.SessionID] = append(bySession[t.SessionID], t.ID)
			} else {
				noSession = append(noSession, t.ID)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: legacy task rows: %w", err)
		}
	}

	// Backfill session-scoped tasks (grouped by session_id).
	for sid, taskIDs := range bySession {
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}

		cid := strings.TrimSpace(existingBySession[sid])
		if cid == "" {
			cid = uuid.NewString()
		}
		toKey := ConversationKey(cid)
		if toKey == "" {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET conversation_id = ?
			WHERE session_id = ? AND (conversation_id IS NULL OR conversation_id = '');
		`, cid, sid); err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: backfill conversation_id for session_id=%q: %w", sid, err)
		}

		// Migrate legacy session-keyed state.
		fromSessionKey := SessionKey("", sid)
		if err := migrateSessionMetaKeyTx(tx, fromSessionKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateSessionWorkspacesKeyTx(tx, fromSessionKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateAcceptanceStateKeyTx(tx, fromSessionKey, toKey, nowMs); err != nil {
			return err
		}

		// Best-effort: migrate any legacy task-keyed state into the same conversation.
		for _, id := range taskIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			fromTaskKey := SessionKey(id, "")
			if err := migrateSessionMetaKeyTx(tx, fromTaskKey, toKey, nowMs); err != nil {
				return err
			}
			if err := migrateSessionWorkspacesKeyTx(tx, fromTaskKey, toKey, nowMs); err != nil {
				return err
			}
			if err := migrateAcceptanceStateKeyTx(tx, fromTaskKey, toKey, nowMs); err != nil {
				return err
			}
		}
	}

	// Backfill tasks without a provider session_id: conversation_id = task_id (deterministic).
	for _, id := range noSession {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cid := id
		toKey := ConversationKey(cid)
		if toKey == "" {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET conversation_id = ?
			WHERE id = ? AND (conversation_id IS NULL OR conversation_id = '');
		`, cid, id); err != nil {
			return fmt.Errorf("tasks: EnsureConversationIDs: backfill conversation_id for task_id=%q: %w", id, err)
		}

		fromKey := SessionKey(id, "")
		if err := migrateSessionMetaKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateSessionWorkspacesKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
		if err := migrateAcceptanceStateKeyTx(tx, fromKey, toKey, nowMs); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tasks: commit EnsureConversationIDs: %w", err)
	}
	return nil
}
