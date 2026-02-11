package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RiskDecisionRecord struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id"`
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	WorkerType     WorkerType      `json:"worker_type"`
	ActionType     string          `json:"action_type"`
	RiskLevel      RiskLevel       `json:"risk_level"`
	Decision       string          `json:"decision"`
	Rationale      string          `json:"rationale"`
	Scope          json.RawMessage `json:"scope"`
	Source         string          `json:"source"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type CreateRiskDecisionInput struct {
	TaskID         string
	SessionID      string
	ConversationID string
	WorkerType     WorkerType
	ActionType     string
	RiskLevel      RiskLevel
	Decision       string
	Rationale      string
	Scope          json.RawMessage
	Source         string
}

type ListRiskDecisionsOptions struct {
	Decision string
	Limit    int
}

func (s *Store) CreateRiskDecision(ctx context.Context, in CreateRiskDecisionInput) (RiskDecisionRecord, error) {
	if s == nil || s.db == nil {
		return RiskDecisionRecord{}, errors.New("tasks: store not initialized")
	}

	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return RiskDecisionRecord{}, errors.New("tasks: task_id is required")
	}

	id := uuid.NewString()
	now := s.now().UTC()
	nowMs := toMillis(now)

	sessionID := strings.TrimSpace(in.SessionID)
	conversationID := strings.TrimSpace(in.ConversationID)
	workerType := strings.TrimSpace(string(in.WorkerType))
	actionType := strings.TrimSpace(in.ActionType)
	riskLevel := strings.TrimSpace(string(in.RiskLevel))
	decision := strings.TrimSpace(in.Decision)
	rationale := strings.TrimSpace(in.Rationale)
	source := strings.TrimSpace(in.Source)
	scope := strings.TrimSpace(string(in.Scope))
	if scope == "" {
		scope = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risk_decisions (
			id, task_id, session_id, conversation_id, worker_type, action_type,
			risk_level, decision, rationale, scope_json, source, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, id, taskID, sessionID, conversationID, workerType, actionType, riskLevel, decision, rationale, scope, source, nowMs, nowMs)
	if err != nil {
		return RiskDecisionRecord{}, fmt.Errorf("tasks: create risk decision: %w", err)
	}

	return RiskDecisionRecord{
		ID:             id,
		TaskID:         taskID,
		SessionID:      sessionID,
		ConversationID: conversationID,
		WorkerType:     WorkerType(workerType),
		ActionType:     actionType,
		RiskLevel:      RiskLevel(riskLevel),
		Decision:       decision,
		Rationale:      rationale,
		Scope:          json.RawMessage(scope),
		Source:         source,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (s *Store) ListRiskDecisionsByTask(ctx context.Context, taskID string, opts ListRiskDecisionsOptions) ([]RiskDecisionRecord, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("tasks: task_id is required")
	}
	return s.listRiskDecisions(ctx, "task_id = ?", taskID, opts)
}

func (s *Store) ListRiskDecisionsBySession(ctx context.Context, sessionID string, opts ListRiskDecisionsOptions) ([]RiskDecisionRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("tasks: session_id is required")
	}
	return s.listRiskDecisions(ctx, "session_id = ?", sessionID, opts)
}

func (s *Store) listRiskDecisions(ctx context.Context, where string, value any, opts ListRiskDecisionsOptions) ([]RiskDecisionRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT
			id, task_id, session_id, conversation_id, worker_type, action_type,
			risk_level, decision, rationale, scope_json, source, created_at, updated_at
		FROM risk_decisions
		WHERE ` + where
	args := []any{value}
	if decision := strings.TrimSpace(opts.Decision); decision != "" {
		query += " AND decision = ?"
		args = append(args, decision)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?;"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list risk decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]RiskDecisionRecord, 0, limit)
	for rows.Next() {
		var (
			rec        RiskDecisionRecord
			workerType string
			riskLevel  string
			scopeJSON  string
			createdAt  int64
			updatedAt  int64
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.SessionID,
			&rec.ConversationID,
			&workerType,
			&rec.ActionType,
			&riskLevel,
			&rec.Decision,
			&rec.Rationale,
			&scopeJSON,
			&rec.Source,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("tasks: scan risk decisions: %w", err)
		}
		rec.ID = strings.TrimSpace(rec.ID)
		rec.TaskID = strings.TrimSpace(rec.TaskID)
		rec.SessionID = strings.TrimSpace(rec.SessionID)
		rec.ConversationID = strings.TrimSpace(rec.ConversationID)
		rec.WorkerType = WorkerType(strings.TrimSpace(workerType))
		rec.ActionType = strings.TrimSpace(rec.ActionType)
		rec.RiskLevel = RiskLevel(strings.TrimSpace(riskLevel))
		rec.Decision = strings.TrimSpace(rec.Decision)
		rec.Rationale = strings.TrimSpace(rec.Rationale)
		rec.Scope = json.RawMessage(strings.TrimSpace(scopeJSON))
		rec.Source = strings.TrimSpace(rec.Source)
		rec.CreatedAt = time.UnixMilli(createdAt).UTC()
		rec.UpdatedAt = time.UnixMilli(updatedAt).UTC()
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list risk decisions rows: %w", err)
	}
	return out, nil
}
