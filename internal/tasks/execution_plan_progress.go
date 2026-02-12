package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExecutionPlanProgress struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Iteration int       `json:"iteration"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

type AppendExecutionPlanProgressInput struct {
	Key       string
	Iteration int
	Action    string
	Status    string
	Summary   string
}

func (s *Store) AppendExecutionPlanProgress(ctx context.Context, in AppendExecutionPlanProgressInput) (ExecutionPlanProgress, error) {
	if s == nil || s.db == nil {
		return ExecutionPlanProgress{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return ExecutionPlanProgress{}, errors.New("tasks: execution plan key is required")
	}
	iteration := in.Iteration
	if iteration < 0 {
		iteration = 0
	}
	action := strings.TrimSpace(in.Action)
	status := strings.TrimSpace(in.Status)
	summary := strings.TrimSpace(in.Summary)
	now := s.now().UTC()
	nowMs := toMillis(now)

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO execution_plan_progress (key, iteration, action, status, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?);
	`, key, iteration, action, status, summary, nowMs)
	if err != nil {
		return ExecutionPlanProgress{}, fmt.Errorf("tasks: append execution plan progress: %w", err)
	}
	id, _ := res.LastInsertId()
	return ExecutionPlanProgress{
		ID:        id,
		Key:       key,
		Iteration: iteration,
		Action:    action,
		Status:    status,
		Summary:   summary,
		CreatedAt: now,
	}, nil
}

func (s *Store) ListExecutionPlanProgress(ctx context.Context, key string, limit int) ([]ExecutionPlanProgress, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("tasks: execution plan key is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, key, iteration, action, status, summary, created_at
		FROM execution_plan_progress
		WHERE key = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?;
	`, key, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list execution plan progress: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ExecutionPlanProgress, 0, limit)
	for rows.Next() {
		var (
			rec       ExecutionPlanProgress
			createdAt int64
		)
		if err := rows.Scan(&rec.ID, &rec.Key, &rec.Iteration, &rec.Action, &rec.Status, &rec.Summary, &createdAt); err != nil {
			return nil, fmt.Errorf("tasks: scan execution plan progress: %w", err)
		}
		rec.Key = strings.TrimSpace(rec.Key)
		rec.Action = strings.TrimSpace(rec.Action)
		rec.Status = strings.TrimSpace(rec.Status)
		rec.Summary = strings.TrimSpace(rec.Summary)
		rec.CreatedAt = fromMillis(createdAt)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list execution plan progress rows: %w", err)
	}
	return out, nil
}
