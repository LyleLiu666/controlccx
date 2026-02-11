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

type RollbackProof struct {
	ID         string          `json:"id"`
	TaskID     string          `json:"task_id"`
	ActionType string          `json:"action_type"`
	ActionRef  string          `json:"action_ref"`
	ProofType  string          `json:"proof_type"`
	ProofRef   string          `json:"proof_ref"`
	Detail     json.RawMessage `json:"detail"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type CreateRollbackProofInput struct {
	TaskID     string
	ActionType string
	ActionRef  string
	ProofType  string
	ProofRef   string
	Detail     json.RawMessage
}

type ListRollbackProofsOptions struct {
	ProofType string
	Limit     int
}

func (s *Store) CreateRollbackProof(ctx context.Context, in CreateRollbackProofInput) (RollbackProof, error) {
	if s == nil || s.db == nil {
		return RollbackProof{}, errors.New("tasks: store not initialized")
	}

	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return RollbackProof{}, errors.New("tasks: task_id is required")
	}

	id := uuid.NewString()
	now := s.now().UTC()
	nowMs := toMillis(now)

	actionType := strings.TrimSpace(in.ActionType)
	actionRef := strings.TrimSpace(in.ActionRef)
	proofType := strings.TrimSpace(in.ProofType)
	proofRef := strings.TrimSpace(in.ProofRef)
	detail := strings.TrimSpace(string(in.Detail))
	if detail == "" {
		detail = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rollback_proofs (
			id, task_id, action_type, action_ref, proof_type, proof_ref, detail_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, id, taskID, actionType, actionRef, proofType, proofRef, detail, nowMs, nowMs)
	if err != nil {
		return RollbackProof{}, fmt.Errorf("tasks: create rollback proof: %w", err)
	}

	return RollbackProof{
		ID:         id,
		TaskID:     taskID,
		ActionType: actionType,
		ActionRef:  actionRef,
		ProofType:  proofType,
		ProofRef:   proofRef,
		Detail:     json.RawMessage(detail),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (s *Store) ListRollbackProofsByTask(ctx context.Context, taskID string, opts ListRollbackProofsOptions) ([]RollbackProof, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("tasks: task_id is required")
	}
	return s.listRollbackProofs(ctx, "task_id = ?", taskID, opts)
}

func (s *Store) ListRollbackProofsByAction(ctx context.Context, taskID string, actionType string, actionRef string, opts ListRollbackProofsOptions) ([]RollbackProof, error) {
	taskID = strings.TrimSpace(taskID)
	actionType = strings.TrimSpace(actionType)
	actionRef = strings.TrimSpace(actionRef)
	if taskID == "" || actionType == "" || actionRef == "" {
		return nil, errors.New("tasks: task_id, action_type and action_ref are required")
	}
	return s.listRollbackProofs(ctx, "task_id = ? AND action_type = ? AND action_ref = ?", []any{taskID, actionType, actionRef}, opts)
}

func (s *Store) listRollbackProofs(ctx context.Context, where string, value any, opts ListRollbackProofsOptions) ([]RollbackProof, error) {
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
		SELECT id, task_id, action_type, action_ref, proof_type, proof_ref, detail_json, created_at, updated_at
		FROM rollback_proofs
		WHERE ` + where
	args := make([]any, 0, 6)
	switch typed := value.(type) {
	case []any:
		args = append(args, typed...)
	default:
		args = append(args, value)
	}
	if proofType := strings.TrimSpace(opts.ProofType); proofType != "" {
		query += " AND proof_type = ?"
		args = append(args, proofType)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?;"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list rollback proofs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]RollbackProof, 0, limit)
	for rows.Next() {
		var (
			rec       RollbackProof
			detail    string
			createdAt int64
			updatedAt int64
		)
		if err := rows.Scan(
			&rec.ID,
			&rec.TaskID,
			&rec.ActionType,
			&rec.ActionRef,
			&rec.ProofType,
			&rec.ProofRef,
			&detail,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("tasks: scan rollback proofs: %w", err)
		}
		rec.ID = strings.TrimSpace(rec.ID)
		rec.TaskID = strings.TrimSpace(rec.TaskID)
		rec.ActionType = strings.TrimSpace(rec.ActionType)
		rec.ActionRef = strings.TrimSpace(rec.ActionRef)
		rec.ProofType = strings.TrimSpace(rec.ProofType)
		rec.ProofRef = strings.TrimSpace(rec.ProofRef)
		rec.Detail = json.RawMessage(strings.TrimSpace(detail))
		rec.CreatedAt = time.UnixMilli(createdAt).UTC()
		rec.UpdatedAt = time.UnixMilli(updatedAt).UTC()
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list rollback proofs rows: %w", err)
	}
	return out, nil
}
