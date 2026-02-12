package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AcceptanceState struct {
	Key           string    `json:"key"`
	Status        string    `json:"status"`
	Iteration     int       `json:"iteration"`
	MaxIterations int       `json:"max_iterations"`
	CurrentGate   string    `json:"current_gate"`
	Summary       string    `json:"summary"`
	PlanJSON      string    `json:"plan_json,omitempty"`
	Report        string    `json:"report,omitempty"`
	RunID         string    `json:"run_id,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UpsertAcceptanceStateInput struct {
	Key           string
	Status        string
	Iteration     int
	MaxIterations int
	CurrentGate   string
	Summary       string
	PlanJSON      string
	Report        string
	RunID         string
}

func (s *Store) GetAcceptanceState(ctx context.Context, key string) (AcceptanceState, bool, error) {
	if s == nil || s.db == nil {
		return AcceptanceState{}, false, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return AcceptanceState{}, false, errors.New("tasks: acceptance key is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT session_key, status, iteration, max_iterations, current_gate, summary, plan_json, report, run_id, updated_at
		FROM acceptance_states
		WHERE session_key = ?;
	`, key)

	var (
		out       AcceptanceState
		updatedAt int64
	)
	if err := row.Scan(
		&out.Key,
		&out.Status,
		&out.Iteration,
		&out.MaxIterations,
		&out.CurrentGate,
		&out.Summary,
		&out.PlanJSON,
		&out.Report,
		&out.RunID,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AcceptanceState{}, false, nil
		}
		return AcceptanceState{}, false, fmt.Errorf("tasks: get acceptance state: %w", err)
	}
	out.UpdatedAt = fromMillis(updatedAt)
	return out, true, nil
}

func (s *Store) UpsertAcceptanceState(ctx context.Context, in UpsertAcceptanceStateInput) (AcceptanceState, error) {
	if s == nil || s.db == nil {
		return AcceptanceState{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return AcceptanceState{}, errors.New("tasks: acceptance key is required")
	}
	maxIterations := in.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 10
	}
	planJSON := strings.TrimSpace(in.PlanJSON)
	if planJSON == "" {
		bridged, ok, err := s.bridgeAcceptancePlanFromMissionContract(ctx, key)
		if err != nil {
			return AcceptanceState{}, err
		}
		if ok {
			planJSON = bridged
		}
	}

	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO acceptance_states (
			session_key, status, iteration, max_iterations, current_gate, summary, plan_json, report, run_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_key) DO UPDATE SET
			status = excluded.status,
			iteration = excluded.iteration,
			max_iterations = excluded.max_iterations,
			current_gate = excluded.current_gate,
			summary = excluded.summary,
			plan_json = excluded.plan_json,
			report = excluded.report,
			run_id = excluded.run_id,
			updated_at = excluded.updated_at;
	`, key, strings.TrimSpace(in.Status), in.Iteration, maxIterations, strings.TrimSpace(in.CurrentGate), strings.TrimSpace(in.Summary), planJSON, strings.TrimSpace(in.Report), strings.TrimSpace(in.RunID), now)
	if err != nil {
		return AcceptanceState{}, fmt.Errorf("tasks: upsert acceptance state: %w", err)
	}

	state, _, err := s.GetAcceptanceState(ctx, key)
	if err != nil {
		return AcceptanceState{}, err
	}
	return state, nil
}

func migrateAcceptanceStateKeyTx(tx *sql.Tx, fromKey, toKey string, nowMs int64) error {
	fromKey = strings.TrimSpace(fromKey)
	toKey = strings.TrimSpace(toKey)
	if fromKey == "" || toKey == "" || fromKey == toKey {
		return nil
	}

	type rowData struct {
		Status        string
		Iteration     int
		MaxIterations int
		CurrentGate   string
		Summary       string
		PlanJSON      string
		Report        string
		RunID         string
		UpdatedAt     int64
	}

	read := func(key string) (rowData, bool, error) {
		var r rowData
		err := tx.QueryRow(`
			SELECT status, iteration, max_iterations, current_gate, summary, plan_json, report, run_id, updated_at
			FROM acceptance_states
			WHERE session_key = ?;
		`, key).Scan(
			&r.Status,
			&r.Iteration,
			&r.MaxIterations,
			&r.CurrentGate,
			&r.Summary,
			&r.PlanJSON,
			&r.Report,
			&r.RunID,
			&r.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return rowData{}, false, nil
			}
			return rowData{}, false, fmt.Errorf("tasks: read acceptance_states(%s): %w", key, err)
		}
		return r, true, nil
	}

	from, ok, err := read(fromKey)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	to, ok, err := read(toKey)
	if err != nil {
		return err
	}
	if !ok {
		_, err := tx.Exec(`UPDATE acceptance_states SET session_key = ?, updated_at = ? WHERE session_key = ?;`, toKey, nowMs, fromKey)
		if err != nil {
			return fmt.Errorf("tasks: migrate acceptance_states key: %w", err)
		}
		return nil
	}

	// Merge into toKey then remove fromKey.
	merged := to
	if from.UpdatedAt > to.UpdatedAt {
		merged = from
	}
	// Fill missing fields from the other row (best-effort; deterministic).
	other := from
	if merged == from {
		other = to
	}
	if strings.TrimSpace(merged.Status) == "" && strings.TrimSpace(other.Status) != "" {
		merged.Status = other.Status
	}
	if merged.Iteration < other.Iteration {
		merged.Iteration = other.Iteration
	}
	if merged.MaxIterations < other.MaxIterations {
		merged.MaxIterations = other.MaxIterations
	}
	if strings.TrimSpace(merged.CurrentGate) == "" && strings.TrimSpace(other.CurrentGate) != "" {
		merged.CurrentGate = other.CurrentGate
	}
	if strings.TrimSpace(merged.Summary) == "" && strings.TrimSpace(other.Summary) != "" {
		merged.Summary = other.Summary
	}
	if strings.TrimSpace(merged.PlanJSON) == "" && strings.TrimSpace(other.PlanJSON) != "" {
		merged.PlanJSON = other.PlanJSON
	}
	if strings.TrimSpace(merged.Report) == "" && strings.TrimSpace(other.Report) != "" {
		merged.Report = other.Report
	}
	if strings.TrimSpace(merged.RunID) == "" && strings.TrimSpace(other.RunID) != "" {
		merged.RunID = other.RunID
	}

	_, err = tx.Exec(`
		UPDATE acceptance_states
		SET status = ?, iteration = ?, max_iterations = ?,
			current_gate = ?, summary = ?, plan_json = ?, report = ?, run_id = ?,
			updated_at = ?
		WHERE session_key = ?;
	`, strings.TrimSpace(merged.Status), merged.Iteration, merged.MaxIterations,
		strings.TrimSpace(merged.CurrentGate), strings.TrimSpace(merged.Summary), strings.TrimSpace(merged.PlanJSON), strings.TrimSpace(merged.Report), strings.TrimSpace(merged.RunID),
		nowMs, toKey,
	)
	if err != nil {
		return fmt.Errorf("tasks: merge acceptance_states: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM acceptance_states WHERE session_key = ?;`, fromKey)
	if err != nil {
		return fmt.Errorf("tasks: delete acceptance_states(from): %w", err)
	}
	return nil
}
