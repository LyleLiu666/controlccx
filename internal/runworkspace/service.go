package runworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"controlccx/internal/tasks"
)

type Service struct {
	store  *tasks.Store
	retain int
}

type Options struct {
	Retain int
}

type EnsureResult struct {
	Workspace tasks.SessionWorkspace
	Logs      []string
}

type MergeResult struct {
	Workspace tasks.SessionWorkspace `json:"workspace"`
	Applied   []string               `json:"applied,omitempty"`
	Conflicts []string               `json:"conflicts,omitempty"`
}

func NewService(store *tasks.Store, opts Options) *Service {
	retain := opts.Retain
	if retain <= 0 {
		retain = 5
	}
	return &Service{
		store:  store,
		retain: retain,
	}
}

func (s *Service) Get(ctx context.Context, key string) (tasks.SessionWorkspace, bool, error) {
	if s == nil || s.store == nil {
		return tasks.SessionWorkspace{}, false, errors.New("runworkspace: store not configured")
	}
	return s.store.GetSessionWorkspace(ctx, key)
}

func (s *Service) EnsureForTask(ctx context.Context, t tasks.Task) (EnsureResult, error) {
	if s == nil || s.store == nil {
		return EnsureResult{}, errors.New("runworkspace: store not configured")
	}

	key := strings.TrimSpace(tasks.SessionKeyForTask(t))
	if key == "" {
		return EnsureResult{}, errors.New("runworkspace: session key is required")
	}
	base := strings.TrimSpace(t.WorkDir)
	if base == "" {
		base = "."
	}
	base = filepath.Clean(base)

	if existing, ok, err := s.store.GetSessionWorkspace(ctx, key); err != nil {
		return EnsureResult{}, err
	} else if ok {
		if existing.RunWorkDir != "" {
			if _, err := os.Stat(existing.RunWorkDir); err == nil {
				// If the workspace was previously merged/discarded, treat new runs as "active" again.
				if strings.TrimSpace(existing.Status) != "active" {
					updated, err := s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
						Key:         existing.Key,
						Kind:        existing.Kind,
						BaseWorkDir: existing.BaseWorkDir,
						RepoRoot:    existing.RepoRoot,
						RunRoot:     existing.RunRoot,
						RunWorkDir:  existing.RunWorkDir,
						BaseBranch:  existing.BaseBranch,
						WorkBranch:  existing.WorkBranch,
						Status:      "active",
					})
					if err == nil {
						existing = updated
					}
				}
				return EnsureResult{Workspace: existing}, nil
			}
		}
	}

	out, err := s.createWorkspace(ctx, key, base, strings.TrimSpace(t.ConversationID))
	if err != nil {
		return EnsureResult{}, err
	}
	return out, nil
}

func (s *Service) Merge(ctx context.Context, key string) (MergeResult, error) {
	if s == nil || s.store == nil {
		return MergeResult{}, errors.New("runworkspace: store not configured")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return MergeResult{}, errors.New("runworkspace: session key is required")
	}

	ws, ok, err := s.store.GetSessionWorkspace(ctx, key)
	if err != nil {
		return MergeResult{}, err
	}
	if !ok {
		return MergeResult{}, errors.New("runworkspace: workspace not found")
	}

	switch strings.TrimSpace(ws.Kind) {
	case "git-worktree":
		if err := mergeGitWorktree(ctx, ws); err != nil {
			return MergeResult{}, err
		}
		updated, err := s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
			Key:         ws.Key,
			Kind:        ws.Kind,
			BaseWorkDir: ws.BaseWorkDir,
			RepoRoot:    ws.RepoRoot,
			RunRoot:     ws.RunRoot,
			RunWorkDir:  ws.RunWorkDir,
			BaseBranch:  ws.BaseBranch,
			WorkBranch:  ws.WorkBranch,
			Status:      "merged",
		})
		if err != nil {
			return MergeResult{}, err
		}
		return MergeResult{Workspace: updated}, nil
	case "copy":
		applied, conflicts, err := applyBackCopyWorkspace(ws)
		if err != nil {
			return MergeResult{}, err
		}
		updated := ws
		if len(conflicts) == 0 {
			if next, err := s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
				Key:         ws.Key,
				Kind:        ws.Kind,
				BaseWorkDir: ws.BaseWorkDir,
				RepoRoot:    ws.RepoRoot,
				RunRoot:     ws.RunRoot,
				RunWorkDir:  ws.RunWorkDir,
				BaseBranch:  ws.BaseBranch,
				WorkBranch:  ws.WorkBranch,
				Status:      "merged",
			}); err == nil {
				updated = next
			}
		}
		return MergeResult{Workspace: updated, Applied: applied, Conflicts: conflicts}, nil
	default:
		return MergeResult{}, fmt.Errorf("runworkspace: unsupported kind %q", strings.TrimSpace(ws.Kind))
	}
}

func (s *Service) Discard(ctx context.Context, key string) error {
	if s == nil || s.store == nil {
		return errors.New("runworkspace: store not configured")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("runworkspace: session key is required")
	}

	ws, ok, err := s.store.GetSessionWorkspace(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	switch strings.TrimSpace(ws.Kind) {
	case "git-worktree":
		if err := removeGitWorktree(ctx, ws); err != nil {
			return err
		}
	case "copy":
		_ = os.RemoveAll(ws.RunRoot)
	default:
		return fmt.Errorf("runworkspace: unsupported kind %q", strings.TrimSpace(ws.Kind))
	}
	_, err = s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
		Key:         ws.Key,
		Kind:        ws.Kind,
		BaseWorkDir: ws.BaseWorkDir,
		RepoRoot:    ws.RepoRoot,
		RunRoot:     ws.RunRoot,
		RunWorkDir:  ws.RunWorkDir,
		BaseBranch:  ws.BaseBranch,
		WorkBranch:  ws.WorkBranch,
		Status:      "discarded",
	})
	return err
}

func (s *Service) createWorkspace(ctx context.Context, key, baseWorkDir, conversationID string) (EnsureResult, error) {
	if s == nil || s.store == nil {
		return EnsureResult{}, errors.New("runworkspace: store not configured")
	}

	var logs []string

	if ws, ok, extra, err := s.tryCreateGitWorktree(ctx, key, baseWorkDir, conversationID); err != nil {
		return EnsureResult{}, err
	} else if ok {
		logs = append(logs, extra...)
		return EnsureResult{Workspace: ws, Logs: logs}, nil
	}

	ws, err := s.createCopyWorkspace(ctx, key, baseWorkDir)
	if err != nil {
		return EnsureResult{}, err
	}
	return EnsureResult{Workspace: ws, Logs: logs}, nil
}

func (s *Service) tryCreateGitWorktree(ctx context.Context, key, baseWorkDir, conversationID string) (tasks.SessionWorkspace, bool, []string, error) {
	repoRoot, ok, err := detectRepoRoot(ctx, baseWorkDir)
	if err != nil || !ok {
		return tasks.SessionWorkspace{}, false, nil, nil
	}

	ws, logs, ok, err := createGitWorktreeWorkspace(ctx, key, baseWorkDir, repoRoot, conversationID)
	if err != nil {
		return tasks.SessionWorkspace{}, false, nil, err
	}
	if !ok {
		return tasks.SessionWorkspace{}, false, nil, nil
	}

	created, err := s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
		Key:         key,
		Kind:        ws.Kind,
		BaseWorkDir: ws.BaseWorkDir,
		RepoRoot:    ws.RepoRoot,
		RunRoot:     ws.RunRoot,
		RunWorkDir:  ws.RunWorkDir,
		BaseBranch:  ws.BaseBranch,
		WorkBranch:  ws.WorkBranch,
		Status:      "active",
	})
	if err != nil {
		return tasks.SessionWorkspace{}, false, nil, err
	}

	if err := s.gcRepoRoot(ctx, created.RepoRoot); err != nil {
		logs = append(logs, fmt.Sprintf("workspace gc warning: %v", err))
	}
	return created, true, logs, nil
}

func (s *Service) createCopyWorkspace(ctx context.Context, key, baseWorkDir string) (tasks.SessionWorkspace, error) {
	id := stableID(key)
	runRoot := filepath.Join(baseWorkDir, ".ccx", "workspaces", id)
	ws, err := createCopyWorkspace(baseWorkDir, runRoot)
	if err != nil {
		return tasks.SessionWorkspace{}, err
	}

	created, err := s.store.UpsertSessionWorkspace(ctx, tasks.UpsertSessionWorkspaceInput{
		Key:         key,
		Kind:        "copy",
		BaseWorkDir: baseWorkDir,
		RepoRoot:    "",
		RunRoot:     ws.RunRoot,
		RunWorkDir:  ws.RunWorkDir,
		BaseBranch:  "",
		WorkBranch:  "",
		Status:      "active",
	})
	if err != nil {
		return tasks.SessionWorkspace{}, err
	}

	if err := s.gcBaseWorkDir(ctx, created.BaseWorkDir); err != nil {
		// Best-effort GC; do not fail run creation.
	}
	return created, nil
}

func stableID(key string) string {
	key = strings.TrimSpace(key)
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func (s *Service) gcRepoRoot(ctx context.Context, repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	list, err := s.store.ListSessionWorkspacesByRepoRoot(ctx, repoRoot, 500)
	if err != nil {
		return err
	}
	return s.gcList(ctx, list)
}

func (s *Service) gcBaseWorkDir(ctx context.Context, baseWorkDir string) error {
	list, err := s.store.ListSessionWorkspacesByBaseWorkdir(ctx, baseWorkDir, 500)
	if err != nil {
		return err
	}
	return s.gcList(ctx, list)
}

func (s *Service) gcList(ctx context.Context, list []tasks.SessionWorkspace) error {
	if s == nil || s.store == nil {
		return errors.New("runworkspace: store not configured")
	}
	keep := s.retain
	if keep <= 0 {
		keep = 5
	}
	if len(list) <= keep {
		return nil
	}

	for i := keep; i < len(list); i++ {
		ws := list[i]
		st := strings.TrimSpace(ws.Status)
		if st != "merged" && st != "discarded" {
			continue
		}

		switch strings.TrimSpace(ws.Kind) {
		case "git-worktree":
			_ = removeGitWorktree(ctx, ws)
		case "copy":
			_ = os.RemoveAll(ws.RunRoot)
		default:
			continue
		}
		_ = s.store.DeleteSessionWorkspace(ctx, ws.Key)
	}
	return nil
}
