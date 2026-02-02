package runworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"controlccx/internal/tasks"

	"github.com/google/uuid"
)

const (
	defaultKeepWorkspaces       = 5
	copyManifestName            = ".ccx-workspace-manifest.json"
	maxHashBytes          int64 = 1 << 20 // 1 MiB
)

type fileSnapshot struct {
	Kind    string `json:"kind"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"mod_time,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Target  string `json:"target,omitempty"`
}

type copyManifest struct {
	BaseWorkDir string                  `json:"base_workdir"`
	Files       map[string]fileSnapshot `json:"files"`
}

type Service struct {
	Store *tasks.Store
	Keep  int
}

func NewService(store *tasks.Store) *Service {
	return &Service{
		Store: store,
		Keep:  defaultKeepWorkspaces,
	}
}

func (s *Service) EnsureForTask(ctx context.Context, task tasks.Task) (tasks.SessionWorkspace, error) {
	if s == nil || s.Store == nil {
		return tasks.SessionWorkspace{}, errors.New("runworkspace: store is required")
	}
	key := taskWorkspaceKey(task)

	if existing, ok, err := s.Store.GetSessionWorkspace(ctx, key); err != nil {
		return tasks.SessionWorkspace{}, err
	} else if ok && existing.Status == tasks.WorkspaceStatusActive {
		return existing, nil
	}
	// Backward-compatible: older runs stored workspaces under session_id/task-id keys.
	// Recover and migrate the mapping to the current (conversation-scoped) key so resume/rehydrate stays consistent.
	if ws, ok, err := s.recoverLegacyWorkspaceForTask(ctx, task, key); err != nil {
		return tasks.SessionWorkspace{}, err
	} else if ok && ws.Status == tasks.WorkspaceStatusActive {
		return ws, nil
	}

	baseWorkDir, err := absClean(task.WorkDir)
	if err != nil {
		return tasks.SessionWorkspace{}, err
	}

	now := time.Now().UTC()
	workspaceID := uuid.NewString()

	repoRoot, okGit := gitRepoRoot(ctx, baseWorkDir)
	scopeRoot := baseWorkDir
	if okGit {
		scopeRoot = repoRoot
		_ = ensureCCXIgnored(repoRoot)
	}

	workspaceRoot := filepath.Join(scopeRoot, ".ccx", "workspaces", workspaceID)
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return tasks.SessionWorkspace{}, fmt.Errorf("runworkspace: create workspace root: %w", err)
	}

	ws := tasks.SessionWorkspace{
		Key:         key,
		WorkspaceID: workspaceID,
		BaseWorkDir: baseWorkDir,
		Status:      tasks.WorkspaceStatusActive,
		CreatedAt:   now,
	}

	if okGit {
		baseBranch, err := gitCurrentBranch(ctx, repoRoot)
		if err != nil {
			okGit = false
		} else {
			if baseBranch == "HEAD" {
				baseBranch = ""
			}
			runRoot := filepath.Join(workspaceRoot, "wt")
			baseRef := baseBranch
			if baseRef == "" {
				baseRef = "HEAD"
			}
			workBranch := "ccx/ws/" + workspaceID

			if err := os.MkdirAll(filepath.Dir(runRoot), 0o755); err != nil {
				return tasks.SessionWorkspace{}, fmt.Errorf("runworkspace: prepare worktree root: %w", err)
			}
			if err := gitWorktreeAdd(ctx, repoRoot, runRoot, workBranch, baseRef); err != nil {
				okGit = false
			} else {
				_, err := copyGitUntracked(ctx, repoRoot, runRoot)
				if err != nil {
					_ = gitWorktreeRemove(ctx, repoRoot, runRoot)
					_ = gitBranchDelete(ctx, repoRoot, workBranch)
					okGit = false
				}

				rel, err := filepath.Rel(repoRoot, baseWorkDir)
				if err != nil {
					rel = "."
				}
				runWorkDir := filepath.Clean(filepath.Join(runRoot, rel))
				if !isWithinDir(runRoot, runWorkDir) {
					runWorkDir = runRoot
				}

				ws.Kind = tasks.WorkspaceKindGitWorktree
				ws.RepoRoot = repoRoot
				ws.RunRoot = runRoot
				ws.RunWorkDir = runWorkDir
				ws.BaseBranch = baseBranch
				ws.WorkBranch = workBranch
			}
		}
	}

	if !okGit {
		runRoot := filepath.Join(workspaceRoot, "copy")
		if err := os.MkdirAll(runRoot, 0o755); err != nil {
			return tasks.SessionWorkspace{}, fmt.Errorf("runworkspace: create copy root: %w", err)
		}
		if err := copyWorkspace(baseWorkDir, runRoot); err != nil {
			return tasks.SessionWorkspace{}, err
		}

		ws.Kind = tasks.WorkspaceKindCopy
		ws.RunRoot = runRoot
		ws.RunWorkDir = runRoot
	}

	out, err := s.Store.UpsertSessionWorkspace(ctx, ws)
	if err != nil {
		return tasks.SessionWorkspace{}, err
	}

	_ = s.gcBestEffort(ctx, scopeRoot)
	return out, nil
}

func taskWorkspaceKey(task tasks.Task) string {
	return tasks.SessionKeyForTask(task)
}

func (s *Service) recoverLegacyWorkspaceForTask(ctx context.Context, task tasks.Task, desiredKey string) (tasks.SessionWorkspace, bool, error) {
	if s == nil || s.Store == nil {
		return tasks.SessionWorkspace{}, false, errors.New("runworkspace: store is required")
	}
	desiredKey = strings.TrimSpace(desiredKey)
	if desiredKey == "" {
		return tasks.SessionWorkspace{}, false, nil
	}

	sessionID := strings.TrimSpace(task.SessionID)
	if sessionID != "" {
		legacySessionKey := tasks.SessionKey("", sessionID)
		if legacySessionKey != desiredKey {
			if ws, ok, err := s.Store.GetSessionWorkspace(ctx, legacySessionKey); err != nil {
				return tasks.SessionWorkspace{}, false, err
			} else if ok {
				if err := s.Store.MigrateSessionWorkspaceKey(ctx, legacySessionKey, desiredKey); err != nil {
					return tasks.SessionWorkspace{}, false, err
				}
				if migrated, ok, err := s.Store.GetSessionWorkspace(ctx, desiredKey); err != nil {
					return tasks.SessionWorkspace{}, false, err
				} else if ok {
					return migrated, true, nil
				}
				return ws, true, nil
			}
		}
	}

	// Task-key legacy mapping (t:<taskID>).
	legacyTaskKey := tasks.SessionKey(task.ID, "")
	if legacyTaskKey != desiredKey {
		if ws, ok, err := s.Store.GetSessionWorkspace(ctx, legacyTaskKey); err != nil {
			return tasks.SessionWorkspace{}, false, err
		} else if ok {
			if err := s.Store.MigrateSessionWorkspaceKey(ctx, legacyTaskKey, desiredKey); err != nil {
				return tasks.SessionWorkspace{}, false, err
			}
			if migrated, ok, err := s.Store.GetSessionWorkspace(ctx, desiredKey); err != nil {
				return tasks.SessionWorkspace{}, false, err
			} else if ok {
				return migrated, true, nil
			}
			return ws, true, nil
		}
	}

	// Legacy scan: workspaces keyed by task id for the same provider session.
	// This is common in older DBs where the session-key migration never ran.
	if sessionID == "" {
		return tasks.SessionWorkspace{}, false, nil
	}

	all, err := s.Store.ListTasksWithOptions(ctx, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return tasks.SessionWorkspace{}, false, err
	}
	for _, t := range all {
		if strings.TrimSpace(t.SessionID) != sessionID {
			continue
		}
		fromKey := tasks.SessionKey(t.ID, "")
		if fromKey == "" || fromKey == desiredKey {
			continue
		}
		ws, ok, err := s.Store.GetSessionWorkspace(ctx, fromKey)
		if err != nil {
			return tasks.SessionWorkspace{}, false, err
		}
		if !ok {
			continue
		}
		// Skip missing workdirs (best-effort; keep searching).
		if strings.TrimSpace(ws.RunWorkDir) != "" {
			if info, err := os.Stat(ws.RunWorkDir); err != nil || !info.IsDir() {
				continue
			}
		}
		if err := s.Store.MigrateSessionWorkspaceKey(ctx, fromKey, desiredKey); err != nil {
			return tasks.SessionWorkspace{}, false, err
		}
		if migrated, ok, err := s.Store.GetSessionWorkspace(ctx, desiredKey); err != nil {
			return tasks.SessionWorkspace{}, false, err
		} else if ok {
			return migrated, true, nil
		}
		return ws, true, nil
	}

	return tasks.SessionWorkspace{}, false, nil
}

func (s *Service) recoverLegacyWorkspaceForSession(ctx context.Context, sessionID string) (tasks.SessionWorkspace, bool, error) {
	if s == nil || s.Store == nil {
		return tasks.SessionWorkspace{}, false, errors.New("runworkspace: store is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return tasks.SessionWorkspace{}, false, nil
	}

	desiredKey := tasks.SessionKey("", sessionID)
	if ws, ok, err := s.Store.GetSessionWorkspace(ctx, desiredKey); err != nil {
		return tasks.SessionWorkspace{}, false, err
	} else if ok {
		return ws, true, nil
	}

	all, err := s.Store.ListTasksWithOptions(ctx, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return tasks.SessionWorkspace{}, false, err
	}

	for _, t := range all {
		if strings.TrimSpace(t.SessionID) != sessionID {
			continue
		}

		legacyKey := tasks.SessionKey(t.ID, "")
		ws, ok, err := s.Store.GetSessionWorkspace(ctx, legacyKey)
		if err != nil {
			return tasks.SessionWorkspace{}, false, err
		}
		if !ok {
			continue
		}

		// Skip missing workdirs (best-effort; keep searching).
		if strings.TrimSpace(ws.RunWorkDir) != "" {
			if info, err := os.Stat(ws.RunWorkDir); err != nil || !info.IsDir() {
				continue
			}
		}

		if err := s.Store.MigrateSessionWorkspaceKey(ctx, legacyKey, desiredKey); err != nil {
			return tasks.SessionWorkspace{}, false, err
		}

		// Prefer the migrated session key mapping.
		if migrated, ok, err := s.Store.GetSessionWorkspace(ctx, desiredKey); err != nil {
			return tasks.SessionWorkspace{}, false, err
		} else if ok {
			return migrated, true, nil
		}

		// If migration didn't stick (race/overwrite avoidance), still return the legacy mapping.
		return ws, true, nil
	}

	return tasks.SessionWorkspace{}, false, nil
}

func absClean(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("runworkspace: abs path: %w", err)
	}
	cleaned := filepath.Clean(abs)
	if real, err := filepath.EvalSymlinks(cleaned); err == nil && strings.TrimSpace(real) != "" {
		cleaned = filepath.Clean(real)
	}
	return cleaned, nil
}

func gitRepoRoot(ctx context.Context, dir string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	root = filepath.Clean(root)
	if real, err := filepath.EvalSymlinks(root); err == nil && strings.TrimSpace(real) != "" {
		root = filepath.Clean(real)
	}
	return root, true
}

func isWithinDir(base, path string) bool {
	base = filepath.Clean(strings.TrimSpace(base))
	path = filepath.Clean(strings.TrimSpace(path))
	if base == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

func ensureCCXIgnored(repoRoot string) error {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	path := filepath.Join(repoRoot, ".git", "info", "exclude")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	raw := string(data)
	if strings.Contains(raw, "\n.ccx/") || strings.HasPrefix(raw, ".ccx/") || strings.Contains(raw, "\r\n.ccx/") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if len(raw) > 0 && !strings.HasSuffix(raw, "\n") {
		if _, err := io.WriteString(f, "\n"); err != nil {
			return err
		}
	}
	_, err = io.WriteString(f, ".ccx/\n")
	return err
}

func gitWorktreeAdd(ctx context.Context, repoRoot, runRoot, branch, ref string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("runworkspace: worktree branch is required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "worktree", "add", "-q", "-b", branch, runRoot, ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runworkspace: git worktree add: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyGitUntracked(ctx context.Context, repoRoot, runRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("runworkspace: git ls-files untracked: %w", err)
	}
	var copied []string
	parts := strings.Split(string(out), "\x00")
	for _, rel := range parts {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		// Avoid recursion if .ccx is not ignored yet.
		if rel == ".ccx" || strings.HasPrefix(rel, ".ccx/") {
			continue
		}

		src := filepath.Join(repoRoot, filepath.FromSlash(rel))
		dst := filepath.Join(runRoot, filepath.FromSlash(rel))
		if err := copyPath(src, dst); err != nil {
			return copied, err
		}
		copied = append(copied, rel)
	}
	return copied, nil
}

func copyWorkspace(baseDir, runRoot string) error {
	ignore := buildIgnoreMatcher(baseDir)
	manifest := copyManifest{
		BaseWorkDir: baseDir,
		Files:       map[string]fileSnapshot{},
	}

	err := filepath.WalkDir(baseDir, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(baseDir, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if ignore != nil && ignore.Ignore(rel, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			manifest.Files[relSlash] = fileSnapshot{Kind: "dir"}
			return nil
		}

		snap, ok, err := fingerprintIfExists(full)
		if err != nil {
			return err
		}
		if ok {
			manifest.Files[relSlash] = snap
		}

		src := full
		dst := filepath.Join(runRoot, rel)
		if err := copyPath(src, dst); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("runworkspace: copy workspace: %w", err)
	}

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("runworkspace: manifest marshal: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, copyManifestName), b, 0o644); err != nil {
		return fmt.Errorf("runworkspace: write manifest: %w", err)
	}
	return nil
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("runworkspace: stat %s: %w", src, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("runworkspace: readlink %s: %w", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("runworkspace: mkdir %s: %w", filepath.Dir(dst), err)
		}
		_ = os.Remove(dst)
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("runworkspace: symlink %s: %w", dst, err)
		}
		return nil
	}
	if info.IsDir() {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("runworkspace: mkdir %s: %w", filepath.Dir(dst), err)
	}
	_ = os.Remove(dst)

	if info.Size() > maxHashBytes && tryCloneFile(src, dst, info.Mode()) {
		_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
		return nil
	}
	if err := copyFile(src, dst, info.Mode()); err != nil {
		return err
	}
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("runworkspace: open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("runworkspace: create %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("runworkspace: copy %s: %w", filepath.Base(src), err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("runworkspace: close %s: %w", dst, err)
	}
	return nil
}

type ignoreMatcher interface {
	Ignore(rel string, isDir bool) bool
}

type ignoreFunc func(rel string, isDir bool) bool

func (f ignoreFunc) Ignore(rel string, isDir bool) bool { return f(rel, isDir) }

func buildIgnoreMatcher(_ string) ignoreMatcher {
	// Keep this conservative: skip only well-known generated directories and our own workspace folder.
	denyDirs := map[string]struct{}{
		".ccx":          {},
		".git":          {},
		"node_modules":  {},
		"dist":          {},
		"build":         {},
		".venv":         {},
		".pytest_cache": {},
		".mypy_cache":   {},
		"__pycache__":   {},
		"target":        {},
		".idea":         {},
		".vscode":       {},
	}
	denyFiles := map[string]struct{}{
		".DS_Store": {},
	}

	return ignoreFunc(func(rel string, _ bool) bool {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" {
			return false
		}
		parts := strings.Split(rel, "/")
		if len(parts) == 0 {
			return false
		}
		if _, ok := denyDirs[parts[0]]; ok {
			return true
		}
		if len(parts) == 1 {
			if _, ok := denyFiles[parts[0]]; ok {
				return true
			}
		}
		return false
	})
}

func (s *Service) gcBestEffort(ctx context.Context, scopeRoot string) error {
	keep := s.Keep
	if keep <= 0 {
		keep = defaultKeepWorkspaces
	}

	all, err := s.Store.ListSessionWorkspaces(ctx, 500)
	if err != nil {
		return err
	}

	var scoped []tasks.SessionWorkspace
	scopeRoot = filepath.Clean(strings.TrimSpace(scopeRoot))
	for _, ws := range all {
		root := strings.TrimSpace(ws.RepoRoot)
		if root == "" {
			root = strings.TrimSpace(ws.BaseWorkDir)
		}
		if filepath.Clean(root) != scopeRoot {
			continue
		}
		scoped = append(scoped, ws)
	}

	if len(scoped) <= keep {
		return nil
	}

	for i := keep; i < len(scoped); i++ {
		ws := scoped[i]
		if ws.Status == tasks.WorkspaceStatusActive {
			continue
		}
		_ = cleanupWorkspace(ctx, ws)
		_ = s.Store.DeleteSessionWorkspace(ctx, ws.Key)
	}
	return nil
}

func cleanupWorkspace(ctx context.Context, ws tasks.SessionWorkspace) error {
	switch ws.Kind {
	case tasks.WorkspaceKindGitWorktree:
		if strings.TrimSpace(ws.RepoRoot) != "" && strings.TrimSpace(ws.RunRoot) != "" {
			_ = gitWorktreeRemove(ctx, ws.RepoRoot, ws.RunRoot)
		}
		if strings.TrimSpace(ws.RepoRoot) != "" && strings.TrimSpace(ws.WorkBranch) != "" {
			_ = gitBranchDelete(ctx, ws.RepoRoot, ws.WorkBranch)
		}
	case tasks.WorkspaceKindCopy:
		if strings.TrimSpace(ws.RunRoot) != "" {
			_ = os.RemoveAll(ws.RunRoot)
		}
	}
	return nil
}

func gitWorktreeRemove(ctx context.Context, repoRoot, runRoot string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "worktree", "remove", "--force", runRoot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runworkspace: git worktree remove: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitBranchDelete(ctx context.Context, repoRoot, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "branch", "-D", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runworkspace: git branch delete: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitRefExists(ctx context.Context, repoRoot, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "show-ref", "--verify", "--quiet", ref)
	return cmd.Run() == nil
}
