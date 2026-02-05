package tasks

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type SessionWorkspace struct {
	Key         string    `json:"key"`
	Kind        string    `json:"kind"`
	BaseWorkDir string    `json:"base_workdir"`
	RepoRoot    string    `json:"repo_root,omitempty"`
	RunRoot     string    `json:"run_root"`
	RunWorkDir  string    `json:"run_workdir"`
	BaseBranch  string    `json:"base_branch,omitempty"`
	WorkBranch  string    `json:"work_branch,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpsertSessionWorkspaceInput struct {
	Key         string
	Kind        string
	BaseWorkDir string
	RepoRoot    string
	RunRoot     string
	RunWorkDir  string
	BaseBranch  string
	WorkBranch  string
	Status      string
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
		SELECT
			key, kind, base_workdir, repo_root, run_root, run_workdir,
			base_branch, work_branch, status,
			created_at, updated_at
		FROM session_workspaces
		WHERE key = ?;
	`, key)

	var (
		out                      SessionWorkspace
		createdAtMs, updatedAtMs int64
	)
	if err := row.Scan(
		&out.Key,
		&out.Kind,
		&out.BaseWorkDir,
		&out.RepoRoot,
		&out.RunRoot,
		&out.RunWorkDir,
		&out.BaseBranch,
		&out.WorkBranch,
		&out.Status,
		&createdAtMs,
		&updatedAtMs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionWorkspace{}, false, nil
		}
		return SessionWorkspace{}, false, fmt.Errorf("tasks: get session workspace: %w", err)
	}
	out.CreatedAt = fromMillis(createdAtMs)
	out.UpdatedAt = fromMillis(updatedAtMs)
	return out, true, nil
}

func (s *Store) UpsertSessionWorkspace(ctx context.Context, in UpsertSessionWorkspaceInput) (SessionWorkspace, error) {
	if s == nil || s.db == nil {
		return SessionWorkspace{}, errors.New("tasks: store not initialized")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		return SessionWorkspace{}, errors.New("tasks: session workspace key is required")
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		return SessionWorkspace{}, errors.New("tasks: session workspace kind is required")
	}

	baseWorkDir := filepath.Clean(strings.TrimSpace(in.BaseWorkDir))
	if baseWorkDir == "" {
		baseWorkDir = "."
	}
	repoRoot := strings.TrimSpace(in.RepoRoot)
	if repoRoot != "" {
		repoRoot = filepath.Clean(repoRoot)
	}

	runRoot := strings.TrimSpace(in.RunRoot)
	if runRoot == "" {
		return SessionWorkspace{}, errors.New("tasks: session workspace run_root and run_workdir are required")
	}
	runRoot = filepath.Clean(runRoot)

	runWorkDir := strings.TrimSpace(in.RunWorkDir)
	if runWorkDir == "" {
		return SessionWorkspace{}, errors.New("tasks: session workspace run_root and run_workdir are required")
	}
	runWorkDir = filepath.Clean(runWorkDir)

	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "active"
	}

	hasWorkspaceID, err := s.sessionWorkspacesHasWorkspaceID(ctx)
	if err != nil {
		return SessionWorkspace{}, err
	}

	nowMs := toMillis(s.now().UTC())
	if hasWorkspaceID {
		workspaceID := legacyWorkspaceID(key, runRoot, runWorkDir)
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO session_workspaces (
				key, workspace_id, kind, base_workdir, repo_root, run_root, run_workdir,
				base_branch, work_branch, status,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET
				kind = excluded.kind,
				base_workdir = excluded.base_workdir,
				repo_root = excluded.repo_root,
				run_root = excluded.run_root,
				run_workdir = excluded.run_workdir,
				base_branch = excluded.base_branch,
				work_branch = excluded.work_branch,
				status = excluded.status,
				updated_at = excluded.updated_at;
		`, key, workspaceID, kind, baseWorkDir, repoRoot, runRoot, runWorkDir,
			strings.TrimSpace(in.BaseBranch), strings.TrimSpace(in.WorkBranch), status,
			nowMs, nowMs,
		)
	} else {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO session_workspaces (
				key, kind, base_workdir, repo_root, run_root, run_workdir,
				base_branch, work_branch, status,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET
				kind = excluded.kind,
				base_workdir = excluded.base_workdir,
				repo_root = excluded.repo_root,
				run_root = excluded.run_root,
				run_workdir = excluded.run_workdir,
				base_branch = excluded.base_branch,
				work_branch = excluded.work_branch,
				status = excluded.status,
				updated_at = excluded.updated_at;
		`, key, kind, baseWorkDir, repoRoot, runRoot, runWorkDir,
			strings.TrimSpace(in.BaseBranch), strings.TrimSpace(in.WorkBranch), status,
			nowMs, nowMs,
		)
	}
	if err != nil {
		return SessionWorkspace{}, fmt.Errorf("tasks: upsert session workspace: %w", err)
	}
	out, _, err := s.GetSessionWorkspace(ctx, key)
	if err != nil {
		return SessionWorkspace{}, err
	}
	return out, nil
}

func (s *Store) sessionWorkspacesHasWorkspaceID(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("tasks: store not initialized")
	}

	s.sessionWorkspacesHasWorkspaceIDMu.Lock()
	if s.sessionWorkspacesHasWorkspaceIDKnown {
		has := s.sessionWorkspacesHasWorkspaceIDValue
		s.sessionWorkspacesHasWorkspaceIDMu.Unlock()
		return has, nil
	}
	s.sessionWorkspacesHasWorkspaceIDMu.Unlock()

	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(session_workspaces);`)
	if err != nil {
		return false, fmt.Errorf("tasks: session_workspaces schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	has := false
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("tasks: session_workspaces schema: %w", err)
		}
		if name == "workspace_id" {
			has = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("tasks: session_workspaces schema: %w", err)
	}

	s.sessionWorkspacesHasWorkspaceIDMu.Lock()
	s.sessionWorkspacesHasWorkspaceIDKnown = true
	s.sessionWorkspacesHasWorkspaceIDValue = has
	s.sessionWorkspacesHasWorkspaceIDMu.Unlock()
	return has, nil
}

func legacyWorkspaceID(key, runRoot, runWorkDir string) string {
	if id := extractWorkspaceID(runRoot); id != "" {
		return id
	}
	if id := extractWorkspaceID(runWorkDir); id != "" {
		return id
	}
	return stableWorkspaceID(key)
}

func extractWorkspaceID(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	slash := filepath.ToSlash(filepath.Clean(path))
	parts := strings.Split(slash, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] != "workspaces" || i+1 >= len(parts) {
			continue
		}
		id := strings.TrimSpace(parts[i+1])
		if id == "" || id == "." {
			continue
		}
		return id
	}
	return ""
}

func stableWorkspaceID(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])[:12]
}

func (s *Store) DeleteSessionWorkspace(ctx context.Context, key string) error {
	if s == nil || s.db == nil {
		return errors.New("tasks: store not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("tasks: session workspace key is required")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM session_workspaces WHERE key = ?;`, key); err != nil {
		return fmt.Errorf("tasks: delete session workspace: %w", err)
	}
	return nil
}

func (s *Store) ListSessionWorkspacesByRepoRoot(ctx context.Context, repoRoot string, limit int) ([]SessionWorkspace, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, errors.New("tasks: repo_root is required")
	}
	repoRoot = filepath.Clean(repoRoot)
	if repoRoot == "" {
		return nil, errors.New("tasks: repo_root is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			key, kind, base_workdir, repo_root, run_root, run_workdir,
			base_branch, work_branch, status,
			created_at, updated_at
		FROM session_workspaces
		WHERE repo_root = ?
		ORDER BY updated_at DESC
		LIMIT ?;
	`, repoRoot, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces by repo_root: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionWorkspace
	for rows.Next() {
		var (
			ws                       SessionWorkspace
			createdAtMs, updatedAtMs int64
		)
		if err := rows.Scan(
			&ws.Key,
			&ws.Kind,
			&ws.BaseWorkDir,
			&ws.RepoRoot,
			&ws.RunRoot,
			&ws.RunWorkDir,
			&ws.BaseBranch,
			&ws.WorkBranch,
			&ws.Status,
			&createdAtMs,
			&updatedAtMs,
		); err != nil {
			return nil, fmt.Errorf("tasks: scan session workspace: %w", err)
		}
		ws.CreatedAt = fromMillis(createdAtMs)
		ws.UpdatedAt = fromMillis(updatedAtMs)
		out = append(out, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces by repo_root rows: %w", err)
	}
	return out, nil
}

func (s *Store) ListSessionWorkspacesByBaseWorkdir(ctx context.Context, baseWorkdir string, limit int) ([]SessionWorkspace, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("tasks: store not initialized")
	}
	baseWorkdir = filepath.Clean(strings.TrimSpace(baseWorkdir))
	if baseWorkdir == "" {
		baseWorkdir = "."
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			key, kind, base_workdir, repo_root, run_root, run_workdir,
			base_branch, work_branch, status,
			created_at, updated_at
		FROM session_workspaces
		WHERE base_workdir = ? AND (repo_root IS NULL OR repo_root = '')
		ORDER BY updated_at DESC
		LIMIT ?;
	`, baseWorkdir, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces by base_workdir: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SessionWorkspace
	for rows.Next() {
		var (
			ws                       SessionWorkspace
			createdAtMs, updatedAtMs int64
		)
		if err := rows.Scan(
			&ws.Key,
			&ws.Kind,
			&ws.BaseWorkDir,
			&ws.RepoRoot,
			&ws.RunRoot,
			&ws.RunWorkDir,
			&ws.BaseBranch,
			&ws.WorkBranch,
			&ws.Status,
			&createdAtMs,
			&updatedAtMs,
		); err != nil {
			return nil, fmt.Errorf("tasks: scan session workspace: %w", err)
		}
		ws.CreatedAt = fromMillis(createdAtMs)
		ws.UpdatedAt = fromMillis(updatedAtMs)
		out = append(out, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list session workspaces by base_workdir rows: %w", err)
	}
	return out, nil
}
