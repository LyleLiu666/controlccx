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
		out        AcceptanceState
		updatedAt  int64
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
	`, key, strings.TrimSpace(in.Status), in.Iteration, maxIterations, strings.TrimSpace(in.CurrentGate), strings.TrimSpace(in.Summary), strings.TrimSpace(in.PlanJSON), strings.TrimSpace(in.Report), strings.TrimSpace(in.RunID), now)
	if err != nil {
		return AcceptanceState{}, fmt.Errorf("tasks: upsert acceptance state: %w", err)
	}

	state, _, err := s.GetAcceptanceState(ctx, key)
	if err != nil {
		return AcceptanceState{}, err
	}
	return state, nil
}

