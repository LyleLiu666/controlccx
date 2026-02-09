package runworkspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"controlccx/internal/tasks"
	"controlccx/internal/worktree"

	"github.com/google/uuid"
)

func detectRepoRoot(ctx context.Context, dir string) (string, bool, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false, nil
	}

	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false, nil
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false, nil
	}
	return filepath.Clean(root), true, nil
}

func createGitWorktreeWorkspace(ctx context.Context, key, baseWorkDir, repoRoot, conversationID string) (tasks.SessionWorkspace, []string, bool, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		// Legacy tasks might have empty conversation_id; derive a stable UUID from the session key.
		conversationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.TrimSpace(key))).String()
	} else if parsed, err := uuid.Parse(conversationID); err == nil {
		conversationID = parsed.String()
	} else {
		// Some legacy migrations use deterministic non-UUID conversation ids (e.g. task_id).
		// Derive a stable UUID so we can safely use the worktree package.
		conversationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(conversationID)).String()
	}

	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	if repoRoot == "" {
		return tasks.SessionWorkspace{}, nil, false, nil
	}

	if err := ensureGitExclude(repoRoot); err != nil {
		// Non-fatal: keep going; worst-case .ccx shows as untracked in base repo.
	}

	baseBranch := ""
	if b, err := gitCurrentBranch(ctx, repoRoot); err == nil {
		baseBranch = b
	}

	var logs []string
	res, err := worktree.Create(ctx, worktree.CreateOptions{
		BaseWorkDir:    baseWorkDir,
		ConversationID: conversationID,
		WorktreeID:     "ws",
		Logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	})
	if err != nil {
		return tasks.SessionWorkspace{}, nil, false, err
	}

	rel := "."
	if r, err := filepath.Rel(repoRoot, baseWorkDir); err == nil {
		r = filepath.Clean(r)
		if r == "." || (r != "" && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))) {
			rel = r
		}
	}

	runWorkDir := strings.TrimSpace(res.Dir)
	if rel != "." {
		runWorkDir = filepath.Join(runWorkDir, rel)
	}

	ws := tasks.SessionWorkspace{
		Key:         key,
		Kind:        "git-worktree",
		BaseWorkDir: filepath.Clean(baseWorkDir),
		RepoRoot:    filepath.Clean(res.RepoRoot),
		RunRoot:     filepath.Clean(res.Dir),
		RunWorkDir:  filepath.Clean(runWorkDir),
		BaseBranch:  strings.TrimSpace(baseBranch),
		WorkBranch:  strings.TrimSpace(res.Branch),
		Status:      "active",
	}
	return ws, logs, true, nil
}

func mergeGitWorktree(ctx context.Context, ws tasks.SessionWorkspace) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("runworkspace: git not found on PATH")
	}

	repoRoot := filepath.Clean(strings.TrimSpace(ws.RepoRoot))
	if repoRoot == "" {
		return errors.New("runworkspace: repo_root is required for git-worktree merge")
	}
	worktreeRoot := filepath.Clean(strings.TrimSpace(ws.RunRoot))
	if worktreeRoot == "" {
		return errors.New("runworkspace: run_root is required for git-worktree merge")
	}
	workBranch := strings.TrimSpace(ws.WorkBranch)
	if workBranch == "" {
		return errors.New("runworkspace: work_branch is required for git-worktree merge")
	}

	if dirty, err := gitIsDirty(ctx, repoRoot); err != nil {
		return err
	} else if dirty {
		return errors.New("runworkspace: base repo has uncommitted changes; please commit/stash before merge")
	}

	if dirty, err := gitIsDirty(ctx, worktreeRoot); err != nil {
		return err
	} else if dirty {
		if err := gitSnapshotCommit(ctx, worktreeRoot); err != nil {
			return err
		}
	}

	target := strings.TrimSpace(ws.BaseBranch)
	if target == "" {
		if b, err := gitCurrentBranch(ctx, repoRoot); err == nil {
			target = b
		}
	}
	if target != "" {
		if out, err := gitCombined(ctx, repoRoot, "checkout", target); err != nil {
			return fmt.Errorf("runworkspace: git checkout %q: %w\n%s", target, err, string(out))
		}
	}

	if out, err := gitCombined(ctx, repoRoot, "merge", "--no-ff", workBranch); err != nil {
		return fmt.Errorf("runworkspace: git merge %q: %w\n%s", workBranch, err, string(out))
	}
	return nil
}

func removeGitWorktree(ctx context.Context, ws tasks.SessionWorkspace) error {
	if _, err := exec.LookPath("git"); err != nil {
		// Best-effort fallback.
		_ = os.RemoveAll(ws.RunRoot)
		return nil
	}

	repoRoot := filepath.Clean(strings.TrimSpace(ws.RepoRoot))
	runRoot := filepath.Clean(strings.TrimSpace(ws.RunRoot))
	if repoRoot == "" || runRoot == "" {
		return nil
	}

	out, err := gitCombined(ctx, repoRoot, "worktree", "remove", "--force", runRoot)
	if err != nil {
		// Fallback cleanup for partially-registered worktrees.
		_ = os.RemoveAll(runRoot)
		return fmt.Errorf("runworkspace: git worktree remove: %w\n%s", err, string(out))
	}
	return nil
}

func ensureGitExclude(repoRoot string) error {
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	if repoRoot == "" {
		return nil
	}
	path := filepath.Join(repoRoot, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	const needle = ".ccx/"
	if b, err := os.ReadFile(path); err == nil {
		if bytes.Contains(b, []byte(needle)) {
			return nil
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString("\n" + needle + "\n"); err != nil {
		return err
	}
	return nil
}

func gitCombined(ctx context.Context, dir string, args ...string) ([]byte, error) {
	base := []string(nil)
	if strings.TrimSpace(dir) != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	return cmd.CombinedOutput()
}

func gitCurrentBranch(ctx context.Context, repoRoot string) (string, error) {
	out, err := gitCombined(ctx, repoRoot, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(out))
	if branch != "" {
		return branch, nil
	}
	out, err = gitCombined(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch = strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return "", nil
	}
	return branch, nil
}

func gitHasHEAD(ctx context.Context, repoRoot string) bool {
	out, err := gitCombined(ctx, repoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func gitIsDirty(ctx context.Context, repoRoot string) (bool, error) {
	out, err := gitCombined(ctx, repoRoot, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("runworkspace: git status: %w\n%s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func gitSnapshotCommit(ctx context.Context, repoRoot string) error {
	if out, err := gitCombined(ctx, repoRoot, "add", "-A"); err != nil {
		return fmt.Errorf("runworkspace: git add: %w\n%s", err, string(out))
	}

	out, err := gitCombined(ctx, repoRoot,
		"-c", "user.email=ccx@local",
		"-c", "user.name=ccx",
		"commit", "--no-gpg-sign", "-m", "ccx: workspace snapshot",
	)
	if err != nil {
		// If there were no changes after add -A, tolerate it.
		msg := strings.ToLower(string(out))
		if strings.Contains(msg, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("runworkspace: git commit snapshot: %w\n%s", err, string(out))
	}
	return nil
}
