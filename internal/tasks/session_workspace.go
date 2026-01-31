package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type WorkspaceKind string

const (
	WorkspaceKindGitWorktree WorkspaceKind = "git-worktree"
	WorkspaceKindCopy        WorkspaceKind = "copy"
)

type WorkspaceStatus string

const (
	WorkspaceStatusActive    WorkspaceStatus = "active"
	WorkspaceStatusMerged    WorkspaceStatus = "merged"
	WorkspaceStatusDiscarded WorkspaceStatus = "discarded"
)

type SessionWorkspace struct {
	Key         string          `json:"key"`
	WorkspaceID string          `json:"workspace_id"`
	Kind        WorkspaceKind   `json:"kind"`
	BaseWorkDir string          `json:"base_workdir"`
	RepoRoot    string          `json:"repo_root,omitempty"`
	RunRoot     string          `json:"run_root"`
	RunWorkDir  string          `json:"run_workdir"`
	BaseBranch  string          `json:"base_branch,omitempty"`
	WorkBranch  string          `json:"work_branch,omitempty"`
	Status      WorkspaceStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func (s *Store) UpsertSessionWorkspace(ctx context.Context, ws SessionWorkspace) (SessionWorkspace, error) {
	if s == nil || s.db == nil {
		return SessionWorkspace{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(ws.Key)
	if key == "" {
		return SessionWorkspace{}, errors.New("tasks: session workspace key is required")
	}
	workspaceID := strings.TrimSpace(ws.WorkspaceID)
	if workspaceID == "" {
		return SessionWorkspace{}, errors.New("tasks: workspace_id is required")
	}
	kind := strings.TrimSpace(string(ws.Kind))
	if kind == "" {
		return SessionWorkspace{}, errors.New("tasks: workspace kind is required")
	}
	baseWorkDir := strings.TrimSpace(ws.BaseWorkDir)
	if baseWorkDir == "" {
		return SessionWorkspace{}, errors.New("tasks: base_workdir is required")
	}
	runRoot := strings.TrimSpace(ws.RunRoot)
	if runRoot == "" {
		return SessionWorkspace{}, errors.New("tasks: run_root is required")
	}
	runWorkDir := strings.TrimSpace(ws.RunWorkDir)
	if runWorkDir == "" {
		return SessionWorkspace{}, errors.New("tasks: run_workdir is required")
	}
	repoRoot := strings.TrimSpace(ws.RepoRoot)
	baseBranch := strings.TrimSpace(ws.BaseBranch)
	workBranch := strings.TrimSpace(ws.WorkBranch)
	status := strings.TrimSpace(string(ws.Status))
	if status == "" {
		status = string(WorkspaceStatusActive)
	}

	now := s.now().UTC()
	createdAt := toMillis(now)
	if !ws.CreatedAt.IsZero() {
		createdAt = toMillis(ws.CreatedAt.UTC())
	}
	updatedAt := toMillis(now)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_workspaces (
			key, workspace_id, kind,
			base_workdir, repo_root,
			run_root, run_workdir,
			base_branch, work_branch,
			status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			workspace_id = excluded.workspace_id,
			kind = excluded.kind,
			base_workdir = excluded.base_workdir,
			repo_root = excluded.repo_root,
			run_root = excluded.run_root,
			run_workdir = excluded.run_workdir,
			base_branch = excluded.base_branch,
			work_branch = excluded.work_branch,
			status = excluded.status,
			updated_at = excluded.updated_at;
	`, key, workspaceID, kind,
		baseWorkDir, repoRoot,
		runRoot, runWorkDir,
		baseBranch, workBranch,
		status, createdAt, updatedAt,
	)
	if err != nil {
		return SessionWorkspace{}, fmt.Errorf("tasks: upsert session workspace: %w", err)
	}

	out, _, err := s.GetSessionWorkspace(ctx, key)
	if err != nil {
		return SessionWorkspace{}, err
	}
	return out, nil
}

func (s *Store) GetSessionWorkspace(ctx context.Context, key string) (SessionWorkspace, bool, error) {
	if s == nil || s.db == nil {
		return SessionWorkspace{}, false, errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return SessionWorkspace{}, false, errors.New("tasks: session workspace key is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT key, workspace_id, kind,
			base_workdir, repo_root,
			run_root, run_workdir,
			base_branch, work_branch,
			status, created_at, updated_at
		FROM session_workspaces
		WHERE key = ?;
	`, key)

	ws, err := scanSessionWorkspace(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionWorkspace{}, false, nil
		}
		return SessionWorkspace{}, false, fmt.Errorf("tasks: get session workspace: %w", err)
	}
	return ws, true, nil
}

func (s *Store) SetSessionWorkspaceStatus(ctx context.Context, key string, status WorkspaceStatus) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("tasks: session workspace key is required")
	}
	st := strings.TrimSpace(string(status))
	if st == "" {
		return errors.New("tasks: workspace status is required")
	}

	now := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		UPDATE session_workspaces
		SET status = ?, updated_at = ?
		WHERE key = ?;
	`, st, now, key)
	if err != nil {
		return fmt.Errorf("tasks: set session workspace status: %w", err)
	}
	return nil
}

func (s *Store) ListSessionWorkspaces(ctx context.Context, limit int) ([]SessionWorkspace, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, workspace_id, kind,
			base_workdir, repo_root,
			run_root, run_workdir,
			base_branch, work_branch,
			status, created_at, updated_at
		FROM session_workspaces
		ORDER BY created_at DESC
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionWorkspace
	for rows.Next() {
		ws, err := scanSessionWorkspace(rows)
		if err != nil {
			return nil, fmt.Errorf("tasks: list session workspaces scan: %w", err)
		}
		out = append(out, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces rows: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteSessionWorkspace(ctx context.Context, key string) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("tasks: session workspace key is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_workspaces WHERE key = ?;`, key)
	if err != nil {
		return fmt.Errorf("tasks: delete session workspace: %w", err)
	}
	return nil
}

type workspaceScanner interface {
	Scan(dest ...any) error
}

func scanSessionWorkspace(row workspaceScanner) (SessionWorkspace, error) {
	var (
		ws                       SessionWorkspace
		kind, status             string
		createdAtMs, updatedAtMs int64
	)
	if err := row.Scan(
		&ws.Key,
		&ws.WorkspaceID,
		&kind,
		&ws.BaseWorkDir,
		&ws.RepoRoot,
		&ws.RunRoot,
		&ws.RunWorkDir,
		&ws.BaseBranch,
		&ws.WorkBranch,
		&status,
		&createdAtMs,
		&updatedAtMs,
	); err != nil {
		return SessionWorkspace{}, err
	}
	ws.Kind = WorkspaceKind(strings.TrimSpace(kind))
	ws.Status = WorkspaceStatus(strings.TrimSpace(status))
	ws.CreatedAt = fromMillis(createdAtMs)
	ws.UpdatedAt = fromMillis(updatedAtMs)
	ws.RepoRoot = strings.TrimSpace(ws.RepoRoot)
	ws.BaseBranch = strings.TrimSpace(ws.BaseBranch)
	ws.WorkBranch = strings.TrimSpace(ws.WorkBranch)
	return ws, nil
}
