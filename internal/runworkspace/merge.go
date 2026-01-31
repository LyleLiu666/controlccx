package runworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"controlccx/internal/tasks"
)

type ConflictError struct {
	Message   string   `json:"message"`
	Conflicts []string `json:"conflicts"`
}

func (e *ConflictError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = "merge conflict"
	}
	if len(e.Conflicts) == 0 {
		return msg
	}
	return fmt.Sprintf("%s (%d files)", msg, len(e.Conflicts))
}

func (s *Service) Merge(ctx context.Context, key string) error {
	if s == nil || s.Store == nil {
		return errors.New("runworkspace: store is required")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("runworkspace: session key is required")
	}

	ws, ok, err := s.Store.GetSessionWorkspace(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("runworkspace: workspace not found")
	}
	if ws.Status == tasks.WorkspaceStatusMerged {
		return nil
	}
	if ws.Status == tasks.WorkspaceStatusDiscarded {
		return errors.New("runworkspace: workspace is discarded")
	}

	switch ws.Kind {
	case tasks.WorkspaceKindGitWorktree:
		if err := mergeGitWorktree(ctx, ws); err != nil {
			return err
		}
	case tasks.WorkspaceKindCopy:
		if err := applyCopyWorkspace(ctx, ws); err != nil {
			return err
		}
	default:
		return fmt.Errorf("runworkspace: unsupported workspace kind %q", ws.Kind)
	}

	if err := s.Store.SetSessionWorkspaceStatus(ctx, ws.Key, tasks.WorkspaceStatusMerged); err != nil {
		return err
	}
	return nil
}

func (s *Service) Discard(ctx context.Context, key string) error {
	if s == nil || s.Store == nil {
		return errors.New("runworkspace: store is required")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("runworkspace: session key is required")
	}

	ws, ok, err := s.Store.GetSessionWorkspace(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("runworkspace: workspace not found")
	}
	if ws.Status == tasks.WorkspaceStatusDiscarded {
		return nil
	}

	switch ws.Kind {
	case tasks.WorkspaceKindGitWorktree:
		if strings.TrimSpace(ws.RepoRoot) != "" && strings.TrimSpace(ws.RunRoot) != "" {
			_ = gitWorktreeRemove(ctx, ws.RepoRoot, ws.RunRoot)
			_ = gitBranchDelete(ctx, ws.RepoRoot, ws.WorkBranch)
		}
	case tasks.WorkspaceKindCopy:
		if strings.TrimSpace(ws.RunRoot) != "" {
			_ = os.RemoveAll(ws.RunRoot)
		}
	}

	if err := s.Store.SetSessionWorkspaceStatus(ctx, ws.Key, tasks.WorkspaceStatusDiscarded); err != nil {
		return err
	}
	return nil
}

func mergeGitWorktree(ctx context.Context, ws tasks.SessionWorkspace) error {
	repoRoot := strings.TrimSpace(ws.RepoRoot)
	runRoot := strings.TrimSpace(ws.RunRoot)
	baseBranch := strings.TrimSpace(ws.BaseBranch)
	workBranch := strings.TrimSpace(ws.WorkBranch)

	if repoRoot == "" || runRoot == "" || baseBranch == "" || workBranch == "" {
		return errors.New("runworkspace: git workspace metadata incomplete")
	}

	// Only auto-merge into a local branch.
	if !gitRefExists(ctx, repoRoot, "refs/heads/"+baseBranch) {
		return fmt.Errorf("runworkspace: base branch %q is not a local branch (cannot auto-merge)", baseBranch)
	}

	if dirty, err := gitStatusDirtyTrackedOnly(ctx, repoRoot); err != nil {
		return err
	} else if dirty {
		return errors.New("runworkspace: base repo has uncommitted changes; commit/stash before merging workspace")
	}

	// Snapshot uncommitted changes in the workspace branch so merge has commits.
	if dirty, err := gitStatusDirty(ctx, runRoot); err != nil {
		return err
	} else if dirty {
		if err := gitCommitWorkspaceSnapshot(ctx, ws, "ccx: workspace snapshot"); err != nil {
			return err
		}
	}

	current, err := gitCurrentBranch(ctx, repoRoot)
	if err != nil {
		return err
	}
	if current != baseBranch {
		if err := gitCheckout(ctx, repoRoot, baseBranch); err != nil {
			return err
		}
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "merge", "--no-ff", "--no-edit", workBranch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(strings.ToLower(msg), "conflict") {
			return &ConflictError{Message: "git merge has conflicts; resolve manually then retry", Conflicts: nil}
		}
		return fmt.Errorf("runworkspace: git merge failed: %s", msg)
	}

	return nil
}

func gitStatusDirty(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("runworkspace: git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func gitStatusDirtyTrackedOnly(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--untracked-files=no")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("runworkspace: git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func gitCurrentBranch(ctx context.Context, repoRoot string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("runworkspace: git current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCheckout(ctx context.Context, repoRoot, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("runworkspace: branch is required")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "checkout", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runworkspace: git checkout %q: %v: %s", branch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitCommitAll(ctx context.Context, repoRoot, message string) error {
	return gitCommitWorkspaceSnapshot(ctx, tasks.SessionWorkspace{RepoRoot: repoRoot, RunRoot: repoRoot}, message)
}

func applyCopyWorkspace(ctx context.Context, ws tasks.SessionWorkspace) error {
	baseDir := strings.TrimSpace(ws.BaseWorkDir)
	runRoot := strings.TrimSpace(ws.RunRoot)
	if baseDir == "" || runRoot == "" {
		return errors.New("runworkspace: copy workspace metadata incomplete")
	}

	manifest, err := readManifest(runRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(manifest.BaseWorkDir) != "" && filepath.Clean(manifest.BaseWorkDir) != filepath.Clean(baseDir) {
		return fmt.Errorf("runworkspace: manifest base_workdir mismatch (got=%q want=%q)", manifest.BaseWorkDir, baseDir)
	}

	ignore := buildIgnoreMatcher(baseDir)
	runFiles, err := listWorkspaceFiles(runRoot, ignore)
	if err != nil {
		return err
	}

	all := make(map[string]struct{}, len(runFiles)+len(manifest.Files))
	for p := range runFiles {
		all[p] = struct{}{}
	}
	for p := range manifest.Files {
		all[p] = struct{}{}
	}

	paths := make([]string, 0, len(all))
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var conflicts []string

	for _, rel := range paths {
		rel = strings.TrimPrefix(rel, "/")
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		if rel == copyManifestName {
			continue
		}
		if ignore != nil && ignore.Ignore(filepath.FromSlash(rel), false) {
			continue
		}

		runPath := filepath.Join(runRoot, filepath.FromSlash(rel))
		basePath := filepath.Join(baseDir, filepath.FromSlash(rel))

		snap, snapOK := manifest.Files[rel]

		runSnap, runOK, err := fingerprintIfExists(runPath)
		if err != nil {
			return err
		}
		baseSnap, baseOK, err := fingerprintIfExists(basePath)
		if err != nil {
			return err
		}

		if !snapOK {
			// New file in workspace.
			if !runOK || runSnap.Kind == "dir" {
				continue
			}
			if baseOK {
				conflicts = append(conflicts, rel)
				continue
			}
			if err := applySnapshotToBase(runPath, basePath, runSnap); err != nil {
				return err
			}
			continue
		}

		// Determine if workspace diverged from snapshot.
		workspaceChanged := !runOK || !snapshotsMatch(runSnap, snap)
		if !workspaceChanged {
			continue
		}

		// Conflict if base changed since snapshot too.
		baseChanged := !baseOK || !snapshotsMatch(baseSnap, snap)
		if baseChanged {
			conflicts = append(conflicts, rel)
			continue
		}

		if !runOK {
			// Deleted in workspace.
			if baseOK {
				if err := os.Remove(basePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("runworkspace: delete %s: %w", rel, err)
				}
			}
			continue
		}
		if runSnap.Kind == "dir" {
			continue
		}
		if err := applySnapshotToBase(runPath, basePath, runSnap); err != nil {
			return err
		}
	}

	if len(conflicts) > 0 {
		return &ConflictError{
			Message:   "copy workspace apply has conflicts; resolve manually",
			Conflicts: conflicts,
		}
	}
	return nil
}

func readManifest(runRoot string) (copyManifest, error) {
	path := filepath.Join(runRoot, copyManifestName)
	b, err := os.ReadFile(path)
	if err != nil {
		return copyManifest{}, fmt.Errorf("runworkspace: read manifest: %w", err)
	}
	var m copyManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return copyManifest{}, fmt.Errorf("runworkspace: parse manifest: %w", err)
	}
	if m.Files == nil {
		m.Files = map[string]fileSnapshot{}
	}
	return m, nil
}

func listWorkspaceFiles(runRoot string, ignore ignoreMatcher) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	err := filepath.WalkDir(runRoot, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(runRoot, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if filepath.Clean(rel) == copyManifestName {
			return nil
		}
		if ignore != nil && ignore.Ignore(rel, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		out[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("runworkspace: list workspace files: %w", err)
	}
	return out, nil
}

func fingerprintIfExists(path string) (fileSnapshot, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileSnapshot{}, false, nil
		}
		return fileSnapshot{}, false, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fileSnapshot{}, false, err
		}
		return fileSnapshot{Kind: "symlink", Target: target}, true, nil
	}
	if info.IsDir() {
		return fileSnapshot{Kind: "dir"}, true, nil
	}

	size := info.Size()
	modTime := info.ModTime().UTC().UnixMilli()
	snap := fileSnapshot{Kind: "file", Size: size, ModTime: modTime}
	if size <= maxHashBytes {
		sum, err := sha256OfFile(path)
		if err != nil {
			return fileSnapshot{}, false, err
		}
		snap.SHA256 = sum
	}
	return snap, true, nil
}

func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func snapshotsMatch(a, b fileSnapshot) bool {
	if a.Kind == "" && b.Kind == "" {
		return true
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case "symlink":
		return a.Target == b.Target
	case "file":
		if a.SHA256 != "" && b.SHA256 != "" {
			return a.SHA256 == b.SHA256
		}
		return a.Size == b.Size && a.ModTime == b.ModTime
	default:
		return true
	}
}

func applySnapshotToBase(src, dst string, snap fileSnapshot) error {
	switch snap.Kind {
	case "symlink":
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		_ = os.Remove(dst)
		if err := os.Symlink(target, dst); err != nil {
			return err
		}
		return nil
	case "file":
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
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
	default:
		return nil
	}
}

func gitCommitWorkspaceSnapshot(ctx context.Context, ws tasks.SessionWorkspace, message string) error {
	repoRoot := strings.TrimSpace(ws.RepoRoot)
	runRoot := strings.TrimSpace(ws.RunRoot)
	if repoRoot == "" || runRoot == "" {
		return errors.New("runworkspace: git workspace metadata incomplete")
	}
	if strings.TrimSpace(message) == "" {
		message = "ccx: workspace snapshot"
	}

	baseUntracked, err := gitUntrackedSet(ctx, repoRoot)
	if err != nil {
		return err
	}

	addU := exec.CommandContext(ctx, "git", "-C", runRoot, "add", "-u")
	if out, err := addU.CombinedOutput(); err != nil {
		return fmt.Errorf("runworkspace: git add -u: %v: %s", err, strings.TrimSpace(string(out)))
	}

	runUntracked, err := gitUntrackedList(ctx, runRoot)
	if err != nil {
		return err
	}
	for _, p := range runUntracked {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == ".ccx" || strings.HasPrefix(p, ".ccx/") {
			continue
		}
		if _, ok := baseUntracked[p]; ok {
			continue
		}
		add := exec.CommandContext(ctx, "git", "-C", runRoot, "add", "--", p)
		if out, err := add.CombinedOutput(); err != nil {
			return fmt.Errorf("runworkspace: git add %s: %v: %s", p, err, strings.TrimSpace(string(out)))
		}
	}

	commit := exec.CommandContext(ctx, "git", "-C", runRoot, "commit", "-m", message)
	out, err := commit.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "nothing to commit") {
			return nil
		}
		if strings.Contains(lower, "user.name") || strings.Contains(lower, "user.email") {
			return fmt.Errorf("runworkspace: git commit failed (missing user.name/user.email): %s", msg)
		}
		return fmt.Errorf("runworkspace: git commit: %v: %s", err, msg)
	}
	return nil
}

func gitUntrackedList(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("runworkspace: git ls-files untracked: %w", err)
	}
	parts := strings.Split(string(out), "\x00")
	files := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		files = append(files, p)
	}
	return files, nil
}

func gitUntrackedSet(ctx context.Context, dir string) (map[string]struct{}, error) {
	files, err := gitUntrackedList(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(files))
	for _, p := range files {
		out[p] = struct{}{}
	}
	return out, nil
}
