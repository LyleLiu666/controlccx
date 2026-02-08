package tasks

import (
	"database/sql"
	"fmt"
	"strings"
)

func migrateSessionWorkspacesKeyTx(tx *sql.Tx, fromKey, toKey string, nowMs int64) error {
	if tx == nil {
		return fmt.Errorf("tasks: migrate session_workspaces key: tx is nil")
	}

	fromKey = strings.TrimSpace(fromKey)
	toKey = strings.TrimSpace(toKey)
	if fromKey == "" || toKey == "" || fromKey == toKey {
		return nil
	}

	var one int
	err := tx.QueryRow(`SELECT 1 FROM session_workspaces WHERE key = ?;`, fromKey).Scan(&one)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("tasks: read session_workspaces(from): %w", err)
	}

	err = tx.QueryRow(`SELECT 1 FROM session_workspaces WHERE key = ?;`, toKey).Scan(&one)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("tasks: read session_workspaces(to): %w", err)
	}

	if err == sql.ErrNoRows {
		_, err = tx.Exec(`UPDATE session_workspaces SET key = ?, updated_at = ? WHERE key = ?;`, toKey, nowMs, fromKey)
		if err != nil {
			return fmt.Errorf("tasks: migrate session_workspaces key: %w", err)
		}
		return nil
	}

	_, err = tx.Exec(`DELETE FROM session_workspaces WHERE key = ?;`, fromKey)
	if err != nil {
		return fmt.Errorf("tasks: delete session_workspaces(from): %w", err)
	}
	return nil
}
