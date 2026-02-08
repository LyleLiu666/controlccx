package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusDenied   ApprovalStatus = "denied"
	ApprovalStatusExpired  ApprovalStatus = "expired"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ApprovalRequest struct {
	ID         string          `json:"id"`
	TaskID     string          `json:"task_id"`
	WorkerType WorkerType      `json:"worker_type"`
	WorkDir    string          `json:"workdir"`
	ActionType string          `json:"action_type"`
	RiskLevel  RiskLevel       `json:"risk_level"`
	Summary    string          `json:"summary"`
	Raw        json.RawMessage `json:"raw"`
	Status     ApprovalStatus  `json:"status"`
	Reason     string          `json:"reason"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type CreateApprovalRequestInput struct {
	TaskID     string
	WorkerType WorkerType
	WorkDir    string
	ActionType string
	RiskLevel  RiskLevel
	Summary    string
	Raw        json.RawMessage
}

func (s *Store) CreateApprovalRequest(ctx context.Context, in CreateApprovalRequestInput) (ApprovalRequest, error) {
	if s == nil || s.db == nil {
		return ApprovalRequest{}, errors.New("tasks: store not initialized")
	}
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return ApprovalRequest{}, errors.New("tasks: task_id is required")
	}

	now := s.now().UTC()
	nowMs := toMillis(now)
	id := uuid.NewString()

	workerType := strings.TrimSpace(string(in.WorkerType))
	workdir := strings.TrimSpace(in.WorkDir)
	actionType := strings.TrimSpace(in.ActionType)
	riskLevel := strings.TrimSpace(string(in.RiskLevel))
	summary := strings.TrimSpace(in.Summary)
	raw := strings.TrimSpace(string(in.Raw))
	if raw == "" {
		raw = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approval_requests (
			id, task_id, worker_type, workdir, action_type, risk_level, summary, raw_json, status, reason, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`, id, taskID, workerType, workdir, actionType, riskLevel, summary, raw, string(ApprovalStatusPending), "", nowMs, nowMs)
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("tasks: create approval request: %w", err)
	}

	out := ApprovalRequest{
		ID:         id,
		TaskID:     taskID,
		WorkerType: WorkerType(workerType),
		WorkDir:    workdir,
		ActionType: actionType,
		RiskLevel:  RiskLevel(riskLevel),
		Summary:    summary,
		Raw:        json.RawMessage(raw),
		Status:     ApprovalStatusPending,
		Reason:     "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return out, nil
}

type ListApprovalRequestsOptions struct {
	Status ApprovalStatus
	Limit  int
}

func (s *Store) ListApprovalRequestsByTask(ctx context.Context, taskID string, opts ListApprovalRequestsOptions) ([]ApprovalRequest, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("tasks: task_id is required")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	args := []any{taskID}
	query := `
		SELECT
			id, task_id, worker_type, workdir, action_type, risk_level, summary, raw_json, status, reason, created_at, updated_at
		FROM approval_requests
		WHERE task_id = ?
	`
	if st := strings.TrimSpace(string(opts.Status)); st != "" {
		query += " AND status = ?"
		args = append(args, st)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?;"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: list approval requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ApprovalRequest
	for rows.Next() {
		var (
			id         string
			tid        string
			workerType string
			workdir    string
			actionType string
			riskLevel  string
			summary    string
			rawJSON    string
			status     string
			reason     string
			createdAt  int64
			updatedAt  int64
		)
		if err := rows.Scan(
			&id,
			&tid,
			&workerType,
			&workdir,
			&actionType,
			&riskLevel,
			&summary,
			&rawJSON,
			&status,
			&reason,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("tasks: scan approval requests: %w", err)
		}
		out = append(out, ApprovalRequest{
			ID:         strings.TrimSpace(id),
			TaskID:     strings.TrimSpace(tid),
			WorkerType: WorkerType(strings.TrimSpace(workerType)),
			WorkDir:    strings.TrimSpace(workdir),
			ActionType: strings.TrimSpace(actionType),
			RiskLevel:  RiskLevel(strings.TrimSpace(riskLevel)),
			Summary:    strings.TrimSpace(summary),
			Raw:        json.RawMessage(strings.TrimSpace(rawJSON)),
			Status:     ApprovalStatus(strings.TrimSpace(status)),
			Reason:     strings.TrimSpace(reason),
			CreatedAt:  time.UnixMilli(createdAt).UTC(),
			UpdatedAt:  time.UnixMilli(updatedAt).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list approval requests rows: %w", err)
	}
	return out, nil
}

type UpdateApprovalRequestDecisionInput struct {
	Status ApprovalStatus
	Reason string
}

type ApprovalNotPendingError struct {
	ApprovalID string
	Status     ApprovalStatus
}

func (e *ApprovalNotPendingError) Error() string {
	id := strings.TrimSpace(e.ApprovalID)
	if id == "" {
		id = "<unknown>"
	}
	st := strings.TrimSpace(string(e.Status))
	if st == "" {
		st = "<unknown>"
	}
	return fmt.Sprintf("tasks: approval request %s is not pending (status=%s)", id, st)
}

func (s *Store) GetApprovalRequest(ctx context.Context, approvalID string) (ApprovalRequest, bool, error) {
	if s == nil || s.db == nil {
		return ApprovalRequest{}, false, errors.New("tasks: store not initialized")
	}
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return ApprovalRequest{}, false, errors.New("tasks: approval_id is required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, task_id, worker_type, workdir, action_type, risk_level, summary, raw_json, status, reason, created_at, updated_at
		FROM approval_requests
		WHERE id = ?;
	`, approvalID)
	var (
		id         string
		tid        string
		workerType string
		workdir    string
		actionType string
		riskLevel  string
		summary    string
		rawJSON    string
		status     string
		reason     string
		createdAt  int64
		updatedAt  int64
	)
	if err := row.Scan(
		&id,
		&tid,
		&workerType,
		&workdir,
		&actionType,
		&riskLevel,
		&summary,
		&rawJSON,
		&status,
		&reason,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ApprovalRequest{}, false, nil
		}
		return ApprovalRequest{}, false, fmt.Errorf("tasks: get approval request: %w", err)
	}
	out := ApprovalRequest{
		ID:         strings.TrimSpace(id),
		TaskID:     strings.TrimSpace(tid),
		WorkerType: WorkerType(strings.TrimSpace(workerType)),
		WorkDir:    strings.TrimSpace(workdir),
		ActionType: strings.TrimSpace(actionType),
		RiskLevel:  RiskLevel(strings.TrimSpace(riskLevel)),
		Summary:    strings.TrimSpace(summary),
		Raw:        json.RawMessage(strings.TrimSpace(rawJSON)),
		Status:     ApprovalStatus(strings.TrimSpace(status)),
		Reason:     strings.TrimSpace(reason),
		CreatedAt:  time.UnixMilli(createdAt).UTC(),
		UpdatedAt:  time.UnixMilli(updatedAt).UTC(),
	}
	return out, true, nil
}

func (s *Store) UpdateApprovalRequestDecision(ctx context.Context, approvalID string, in UpdateApprovalRequestDecisionInput) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return errors.New("tasks: approval_id is required")
	}
	status := ApprovalStatus(strings.TrimSpace(string(in.Status)))
	switch status {
	case ApprovalStatusApproved, ApprovalStatusDenied, ApprovalStatusExpired:
		// ok
	default:
		return fmt.Errorf("tasks: invalid approval status %q", status)
	}

	reason := strings.TrimSpace(in.Reason)
	now := toMillis(s.now().UTC())
	res, err := s.db.ExecContext(ctx, `
		UPDATE approval_requests
		SET status = ?, reason = ?, updated_at = ?
		WHERE id = ? AND status = ?;
	`, string(status), reason, now, approvalID, string(ApprovalStatusPending))
	if err != nil {
		return fmt.Errorf("tasks: update approval decision: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		existing, ok, err := s.GetApprovalRequest(ctx, approvalID)
		if err != nil {
			return err
		}
		if !ok {
			return sql.ErrNoRows
		}
		return &ApprovalNotPendingError{
			ApprovalID: approvalID,
			Status:     existing.Status,
		}
	}
	return nil
}
