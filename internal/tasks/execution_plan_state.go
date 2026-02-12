package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExecutionPlanState struct {
	Key             string    `json:"key"`
	MissionRevision int       `json:"mission_revision"`
	PlanJSON        string    `json:"plan_json"`
	Iteration       int       `json:"iteration"`
	LastAction      string    `json:"last_action"`
	LastTaskID      string    `json:"last_task_id"`
	Status          string    `json:"status"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UpsertExecutionPlanStateInput struct {
	Key             string
	MissionRevision int
	PlanJSON        string
	Iteration       int
	LastAction      string
	LastTaskID      string
	Status          string
}

func (s *Store) GetExecutionPlanState(ctx context.Context, key string) (ExecutionPlanState, bool, error) {
	if s == nil || s.db == nil {
		return ExecutionPlanState{}, false, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ExecutionPlanState{}, false, errors.New("tasks: execution plan key is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT key, mission_revision, plan_json, iteration, last_action, last_task_id, status, updated_at
		FROM execution_plan_states
		WHERE key = ?;
	`, key)

	var (
		out         ExecutionPlanState
		updatedAtMs int64
	)
	if err := row.Scan(
		&out.Key,
		&out.MissionRevision,
		&out.PlanJSON,
		&out.Iteration,
		&out.LastAction,
		&out.LastTaskID,
		&out.Status,
		&updatedAtMs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExecutionPlanState{}, false, nil
		}
		return ExecutionPlanState{}, false, fmt.Errorf("tasks: get execution plan state: %w", err)
	}
	out.Key = strings.TrimSpace(out.Key)
	out.PlanJSON = strings.TrimSpace(out.PlanJSON)
	out.LastAction = strings.TrimSpace(out.LastAction)
	out.LastTaskID = strings.TrimSpace(out.LastTaskID)
	out.Status = strings.TrimSpace(out.Status)
	out.UpdatedAt = fromMillis(updatedAtMs)
	return out, true, nil
}

func (s *Store) UpsertExecutionPlanState(ctx context.Context, in UpsertExecutionPlanStateInput) (ExecutionPlanState, error) {
	if s == nil || s.db == nil {
		return ExecutionPlanState{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return ExecutionPlanState{}, errors.New("tasks: execution plan key is required")
	}

	missionRevision := in.MissionRevision
	if missionRevision < 0 {
		missionRevision = 0
	}
	iteration := in.Iteration
	if iteration < 0 {
		iteration = 0
	}
	nowMs := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO execution_plan_states (
			key, mission_revision, plan_json, iteration, last_action, last_task_id, status, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			mission_revision = excluded.mission_revision,
			plan_json = excluded.plan_json,
			iteration = excluded.iteration,
			last_action = excluded.last_action,
			last_task_id = excluded.last_task_id,
			status = excluded.status,
			updated_at = excluded.updated_at;
	`, key, missionRevision, strings.TrimSpace(in.PlanJSON), iteration, strings.TrimSpace(in.LastAction), strings.TrimSpace(in.LastTaskID), strings.TrimSpace(in.Status), nowMs)
	if err != nil {
		return ExecutionPlanState{}, fmt.Errorf("tasks: upsert execution plan state: %w", err)
	}
	out, _, err := s.GetExecutionPlanState(ctx, key)
	if err != nil {
		return ExecutionPlanState{}, err
	}
	return out, nil
}
