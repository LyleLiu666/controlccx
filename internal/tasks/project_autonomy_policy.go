package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	AutonomyModeGraded = "graded"
	AutonomyModeMax    = "max"
)

type ProjectAutonomyPolicy struct {
	ProjectKey string    `json:"project_key"`
	Mode       string    `json:"mode"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NormalizeProjectKey(workdir string) string {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return ""
	}
	cleaned := filepath.Clean(workdir)
	return filepath.ToSlash(strings.TrimSpace(cleaned))
}

func (s *Store) GetProjectAutonomyPolicy(ctx context.Context, projectKey string) (ProjectAutonomyPolicy, error) {
	if s == nil || s.db == nil {
		return ProjectAutonomyPolicy{}, errors.New("tasks: store not initialized")
	}
	projectKey = NormalizeProjectKey(projectKey)
	if projectKey == "" {
		return ProjectAutonomyPolicy{}, errors.New("tasks: project_key is required")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT project_key, mode, updated_at
		FROM project_autonomy_policies
		WHERE project_key = ?;
	`, projectKey)
	var (
		out         ProjectAutonomyPolicy
		updatedAtMs int64
	)
	if err := row.Scan(&out.ProjectKey, &out.Mode, &updatedAtMs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectAutonomyPolicy{
				ProjectKey: projectKey,
				Mode:       AutonomyModeGraded,
			}, nil
		}
		return ProjectAutonomyPolicy{}, fmt.Errorf("tasks: get project autonomy policy: %w", err)
	}
	out.ProjectKey = NormalizeProjectKey(out.ProjectKey)
	mode, ok := normalizeAutonomyMode(out.Mode)
	if !ok {
		mode = AutonomyModeGraded
	}
	out.Mode = mode
	out.UpdatedAt = fromMillis(updatedAtMs)
	return out, nil
}

func (s *Store) UpsertProjectAutonomyPolicy(ctx context.Context, projectKey, mode string) (ProjectAutonomyPolicy, error) {
	if s == nil || s.db == nil {
		return ProjectAutonomyPolicy{}, errors.New("tasks: store not initialized")
	}
	projectKey = NormalizeProjectKey(projectKey)
	if projectKey == "" {
		return ProjectAutonomyPolicy{}, errors.New("tasks: project_key is required")
	}
	mode, ok := normalizeAutonomyMode(mode)
	if !ok {
		return ProjectAutonomyPolicy{}, errors.New("tasks: invalid autonomy mode")
	}

	nowMs := toMillis(s.now().UTC())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_autonomy_policies (project_key, mode, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_key) DO UPDATE SET
			mode = excluded.mode,
			updated_at = excluded.updated_at;
	`, projectKey, mode, nowMs)
	if err != nil {
		return ProjectAutonomyPolicy{}, fmt.Errorf("tasks: upsert project autonomy policy: %w", err)
	}
	return s.GetProjectAutonomyPolicy(ctx, projectKey)
}

func normalizeAutonomyMode(mode string) (string, bool) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", AutonomyModeGraded:
		return AutonomyModeGraded, true
	case AutonomyModeMax:
		return AutonomyModeMax, true
	default:
		return "", false
	}
}

