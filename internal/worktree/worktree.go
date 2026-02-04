package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type Result struct {
	RepoRoot       string
	BaseWorkDir    string
	Dir            string
	Branch         string
	UntrackedFiles int
	PatchBytes     int
}

const maxOptionalConfigFileBytes = 1 << 20 // 1 MiB

type CreateOptions struct {
	// BaseWorkDir is the original workdir the user selected (within a git repo).
	BaseWorkDir string
	// ConversationID is used to group worktrees under `.ccx/worktrees/<conversation_id>/`.
	ConversationID string
	// WorktreeID is an optional unique identifier for this worktree. If empty, a UUID is generated.
	WorktreeID string
}

func Create(ctx context.Context, opts CreateOptions) (Result, error) {
	base := strings.TrimSpace(opts.BaseWorkDir)
	if base == "" {
		base = "."
	}
	base = filepath.Clean(base)

	cid := strings.TrimSpace(opts.ConversationID)
	if cid == "" {
		return Result{}, errors.New("worktree: conversation_id is required")
	}

	wid := strings.TrimSpace(opts.WorktreeID)
	if wid == "" {
		wid = uuid.NewString()
	}
	short := wid
	if len(short) > 12 {
		short = short[:12]
	}

	repoRoot, err := gitRepoRoot(ctx, base)
	if err != nil {
		return Result{}, err
	}
	repoRoot = filepath.Clean(repoRoot)
	if repoRoot == "" {
		return Result{}, errors.New("worktree: empty repo root")
	}

	worktreesRoot := filepath.Join(repoRoot, ".ccx", "worktrees")
	dir := filepath.Join(worktreesRoot, cid, short)
	if !isWithinRoot(dir, worktreesRoot) {
		return Result{}, fmt.Errorf("worktree: invalid conversation_id %q", cid)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return Result{}, fmt.Errorf("worktree: mkdir parents: %w", err)
	}

	branch := fmt.Sprintf("ccx/%s/%s", cid, short)
	if err := gitWorktreeAdd(ctx, repoRoot, dir, branch); err != nil {
		return Result{}, err
	}

	patch, err := gitDiffHeadPatch(ctx, repoRoot)
	if err != nil {
		return Result{}, err
	}
	if len(patch) > 0 {
		if err := gitApplyPatch(ctx, dir, patch); err != nil {
			return Result{}, err
		}
	}

	untracked, err := gitListUntracked(ctx, repoRoot)
	if err != nil {
		return Result{}, err
	}
	for _, rel := range untracked {
		src, err := resolveWithin(repoRoot, rel)
		if err != nil {
			continue
		}
		dst, err := resolveWithin(dir, rel)
		if err != nil {
			continue
		}
		if err := copyPath(src, dst); err != nil {
			return Result{}, err
		}
	}

	if err := copyOptionalIgnoredConfigFiles(repoRoot, dir); err != nil {
		return Result{}, err
	}

	return Result{
		RepoRoot:       repoRoot,
		BaseWorkDir:    base,
		Dir:            dir,
		Branch:         branch,
		UntrackedFiles: len(untracked),
		PatchBytes:     len(patch),
	}, nil
}

func copyOptionalIgnoredConfigFiles(repoRoot, worktreeDir string) error {
	patterns := []string{".env", ".env.*"}
	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pat))
		if err != nil {
			continue
		}
		for _, srcAbs := range matches {
			rel, err := filepath.Rel(repoRoot, srcAbs)
			if err != nil {
				continue
			}
			src, err := resolveWithin(repoRoot, rel)
			if err != nil {
				continue
			}
			dst, err := resolveWithin(worktreeDir, rel)
			if err != nil {
				continue
			}

			info, err := os.Lstat(src)
			if err != nil {
				continue
			}
			if info.IsDir() {
				continue
			}
			if info.Mode().IsRegular() && info.Size() > maxOptionalConfigFileBytes {
				continue
			}

			if err := copyPath(src, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

func gitRepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("worktree: resolve git root: %w", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errors.New("worktree: git root not found")
	}
	return root, nil
}

func gitWorktreeAdd(ctx context.Context, repoRoot, dir, branch string) error {
	dir = filepath.Clean(dir)
	if dir == "" {
		return errors.New("worktree: invalid worktree dir")
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("worktree: invalid worktree branch")
	}

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("worktree: target exists: %s", dir)
	}

	// Note: we create a new branch to make merge-back deterministic.
	if _, err := gitOutput(ctx, repoRoot, "worktree", "add", "-b", branch, dir); err != nil {
		return fmt.Errorf("worktree: git worktree add: %w", err)
	}
	return nil
}

func gitDiffHeadPatch(ctx context.Context, repoRoot string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--binary", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("worktree: git diff: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("worktree: git diff: %w", err)
	}
	return out, nil
}

func gitApplyPatch(ctx context.Context, worktreeDir string, patch []byte) error {
	if len(patch) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", worktreeDir, "apply", "--whitespace=nowarn", "--recount", "-")
	cmd.Stdin = bytes.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: git apply: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitListUntracked(ctx context.Context, repoRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("worktree: git ls-files: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("worktree: git ls-files: %w", err)
	}

	if len(out) == 0 {
		return nil, nil
	}
	parts := strings.Split(string(out), "\x00")
	var files []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return nil, nil
	}
	return files, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	base := []string(nil)
	if strings.TrimSpace(dir) != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	b, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(b), nil
}

func resolveWithin(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return filepath.Clean(root), nil
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	if cleaned == "" || cleaned == "." {
		return filepath.Clean(root), nil
	}
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("worktree: invalid path %q", rel)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("worktree: invalid path %q", rel)
	}
	joined := filepath.Clean(filepath.Join(root, cleaned))
	if !isWithinRoot(joined, root) {
		return "", fmt.Errorf("worktree: path escapes root: %q", rel)
	}
	return joined, nil
}

func isWithinRoot(p, root string) bool {
	p = filepath.Clean(p)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("worktree: stat %s: %w", src, err)
	}
	if info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("worktree: mkdir %s: %w", filepath.Dir(dst), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("worktree: readlink %s: %w", src, err)
		}
		_ = os.Remove(dst)
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("worktree: symlink %s: %w", dst, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("worktree: open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode()&0o777)
	if err != nil {
		return fmt.Errorf("worktree: create %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("worktree: copy %s: %w", dst, err)
	}
	return nil
}
