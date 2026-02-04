package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

type UntrackedMode string

const (
	UntrackedModeDefault UntrackedMode = ""
	UntrackedModeSkip    UntrackedMode = "skip"
	UntrackedModeForce   UntrackedMode = "force"
)

type UntrackedLargest struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

type UntrackedTooLargeError struct {
	Files    int
	Bytes    int64
	MaxFiles int
	MaxBytes int64
	Largest  []UntrackedLargest
}

func (e *UntrackedTooLargeError) Error() string {
	if e == nil {
		return "worktree: untracked copy too large"
	}
	return fmt.Sprintf(
		"worktree: untracked copy too large: %d files, %d bytes (limits: %d files, %d bytes)",
		e.Files,
		e.Bytes,
		e.MaxFiles,
		e.MaxBytes,
	)
}

const (
	maxOptionalConfigFileBytes = 1 << 20 // 1 MiB

	defaultMaxUntrackedFiles = 2000
	defaultMaxUntrackedBytes = 20 * 1024 * 1024

	gitLockRetryMaxElapsed   = 8 * time.Second
	gitLockRetryInitialDelay = 80 * time.Millisecond
	gitLockRetryMaxDelay     = 800 * time.Millisecond
)

var defaultExcludedUntrackedDirs = map[string]bool{
	".venv":       true,
	".ccx":        true,
	".git":        true,
	"node_modules": true,
}

type Logf func(format string, args ...any)

type CreateOptions struct {
	// BaseWorkDir is the original workdir the user selected (within a git repo).
	BaseWorkDir string
	// ConversationID is used to group worktrees under `.ccx/worktrees/<conversation_id>/`.
	ConversationID string
	// WorktreeID is an optional unique identifier for this worktree. If empty, a UUID is generated.
	WorktreeID string
	// Untracked controls how untracked files are copied into the worktree.
	//
	// Default behavior is to copy untracked files (except excluded heavy dirs),
	// but return an *UntrackedTooLargeError when caps are exceeded.
	Untracked UntrackedMode
	// UntrackedMaxFiles overrides the default max files cap for untracked copy (0 uses default).
	UntrackedMaxFiles int
	// UntrackedMaxBytes overrides the default max bytes cap for untracked copy (0 uses default).
	UntrackedMaxBytes int64
	// Logf optionally receives progress messages (e.g. git lock retry).
	Logf Logf
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
	parsedCID, err := uuid.Parse(cid)
	if err != nil {
		return Result{}, errors.New("worktree: conversation_id must be a UUID")
	}
	cid = parsedCID.String()

	wid := strings.TrimSpace(opts.WorktreeID)
	if wid == "" {
		wid = uuid.NewString()
	}
	short := wid
	if len(short) > 12 {
		short = short[:12]
	}

	repoRoot, err := gitRepoRoot(ctx, base, opts.Logf)
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
	branch := fmt.Sprintf("ccx/%s/%s", cid, short)

	untrackedAll, err := gitListUntracked(ctx, repoRoot, opts.Logf)
	if err != nil {
		return Result{}, err
	}
	untrackedCopy := filterUntracked(untrackedAll)

	maxFiles := opts.UntrackedMaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxUntrackedFiles
	}
	maxBytes := opts.UntrackedMaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxUntrackedBytes
	}

	stats, err := untrackedStats(repoRoot, untrackedCopy)
	if err != nil {
		return Result{}, err
	}

	switch opts.Untracked {
	case UntrackedModeSkip:
		untrackedCopy = nil
		stats = untrackedStatsResult{}
	case UntrackedModeDefault, UntrackedModeForce:
		// ok
	default:
		return Result{}, fmt.Errorf("worktree: invalid untracked mode %q", strings.TrimSpace(string(opts.Untracked)))
	}

	if opts.Untracked == UntrackedModeDefault && (stats.Files > maxFiles || stats.Bytes > maxBytes) {
		return Result{}, &UntrackedTooLargeError{
			Files:    stats.Files,
			Bytes:    stats.Bytes,
			MaxFiles: maxFiles,
			MaxBytes: maxBytes,
			Largest:  stats.Largest,
		}
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return Result{}, fmt.Errorf("worktree: mkdir parents: %w", err)
	}

	if err := gitWorktreeAdd(ctx, repoRoot, dir, branch, opts.Logf); err != nil {
		return Result{}, err
	}

	patch, err := gitDiffHeadPatch(ctx, repoRoot, opts.Logf)
	if err != nil {
		return Result{}, err
	}
	if len(patch) > 0 {
		if err := gitApplyPatch(ctx, dir, patch, opts.Logf); err != nil {
			return Result{}, err
		}
	}

	for _, rel := range untrackedCopy {
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
		UntrackedFiles: len(untrackedCopy),
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

func gitRepoRoot(ctx context.Context, dir string, logf Logf) (string, error) {
	out, err := gitOutputWithRetry(ctx, dir, logf, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("worktree: resolve git root: %w", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errors.New("worktree: git root not found")
	}
	return root, nil
}

func gitWorktreeAdd(ctx context.Context, repoRoot, dir, branch string, logf Logf) error {
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
	if _, err := gitOutputWithRetry(ctx, repoRoot, logf, "worktree", "add", "-b", branch, dir); err != nil {
		return fmt.Errorf("worktree: git worktree add: %w", err)
	}
	return nil
}

func gitDiffHeadPatch(ctx context.Context, repoRoot string, logf Logf) ([]byte, error) {
	out, err := gitOutputBytesWithRetry(ctx, repoRoot, logf, "diff", "--binary", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("worktree: git diff: %w", err)
	}
	return out, nil
}

func gitApplyPatch(ctx context.Context, worktreeDir string, patch []byte, logf Logf) error {
	if len(patch) == 0 {
		return nil
	}
	return gitApplyPatchWithRetry(ctx, worktreeDir, patch, logf)
}

func gitListUntracked(ctx context.Context, repoRoot string, logf Logf) ([]string, error) {
	out, err := gitOutputBytesWithRetry(ctx, repoRoot, logf, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
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

func gitOutputBytes(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	base := []string(nil)
	if strings.TrimSpace(dir) != "" {
		base = append(base, "-C", dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, ee.Stderr, err
		}
		return out, nil, err
	}
	return out, nil, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, stderr, err := gitOutputBytes(ctx, dir, args...)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return string(out), nil
}

func gitOutputWithRetry(ctx context.Context, dir string, logf Logf, args ...string) (string, error) {
	attempt := 0
	start := time.Now()
	delay := gitLockRetryInitialDelay
	for {
		attempt++
		out, err := gitOutput(ctx, dir, args...)
		if err == nil {
			return out, nil
		}
		msg := strings.TrimSpace(err.Error())
		if !isGitLockErrorMessage(msg) {
			return "", err
		}
		if time.Since(start) >= gitLockRetryMaxElapsed {
			return "", fmt.Errorf("%s", msg)
		}
		if logf != nil {
			logf("git lock 重试中…（第 %d 次）", attempt)
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return "", err
		}
		delay = time.Duration(math.Min(float64(delay*2), float64(gitLockRetryMaxDelay)))
	}
}

func gitOutputBytesWithRetry(ctx context.Context, dir string, logf Logf, args ...string) ([]byte, error) {
	attempt := 0
	start := time.Now()
	delay := gitLockRetryInitialDelay
	for {
		attempt++
		out, stderr, err := gitOutputBytes(ctx, dir, args...)
		if err == nil {
			return out, nil
		}
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(out))
		}
		if msg == "" {
			msg = err.Error()
		}
		if !isGitLockErrorMessage(msg) {
			return nil, fmt.Errorf("%s", msg)
		}
		if time.Since(start) >= gitLockRetryMaxElapsed {
			return nil, fmt.Errorf("%s", msg)
		}
		if logf != nil {
			logf("git lock 重试中…（第 %d 次）", attempt)
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, err
		}
		delay = time.Duration(math.Min(float64(delay*2), float64(gitLockRetryMaxDelay)))
	}
}

func gitApplyPatchWithRetry(ctx context.Context, worktreeDir string, patch []byte, logf Logf) error {
	attempt := 0
	start := time.Now()
	delay := gitLockRetryInitialDelay
	for {
		attempt++
		cmd := exec.CommandContext(ctx, "git", "-C", worktreeDir, "apply", "--whitespace=nowarn", "--recount", "-")
		cmd.Stdin = bytes.NewReader(patch)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if !isGitLockErrorMessage(msg) {
			return fmt.Errorf("worktree: git apply: %s", msg)
		}
		if time.Since(start) >= gitLockRetryMaxElapsed {
			return fmt.Errorf("worktree: git apply: %s", msg)
		}
		if logf != nil {
			logf("git lock 重试中…（第 %d 次）", attempt)
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return err
		}
		delay = time.Duration(math.Min(float64(delay*2), float64(gitLockRetryMaxDelay)))
	}
}

func isGitLockErrorMessage(msg string) bool {
	s := strings.ToLower(strings.TrimSpace(msg))
	if s == "" {
		return false
	}
	if strings.Contains(s, "another git process") {
		return true
	}
	if strings.Contains(s, "index.lock") || strings.Contains(s, "packed-refs.lock") {
		return true
	}
	if strings.Contains(s, "cannot lock ref") || strings.Contains(s, "unable to lock") || strings.Contains(s, "could not lock") {
		return true
	}
	if strings.Contains(s, "unable to create") && strings.Contains(s, ".lock") && strings.Contains(s, "exists") {
		return true
	}
	if strings.Contains(s, "lock file") && strings.Contains(s, "exists") {
		return true
	}
	return false
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type untrackedStatsResult struct {
	Files   int
	Bytes   int64
	Largest []UntrackedLargest
}

func filterUntracked(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if shouldExcludeUntracked(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func shouldExcludeUntracked(rel string) bool {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return true
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	parts := strings.Split(rel, "/")
	if len(parts) <= 1 {
		return false
	}
	for _, seg := range parts[:len(parts)-1] {
		if defaultExcludedUntrackedDirs[seg] {
			return true
		}
	}
	return false
}

func untrackedStats(repoRoot string, rels []string) (untrackedStatsResult, error) {
	if len(rels) == 0 {
		return untrackedStatsResult{}, nil
	}
	var largest []UntrackedLargest
	files := 0
	var bytes int64
	for _, rel := range rels {
		src, err := resolveWithin(repoRoot, rel)
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
		if info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
			continue
		}
		size := info.Size()
		files++
		bytes += size
		if size <= 0 {
			continue
		}
		largest = append(largest, UntrackedLargest{Path: rel, Bytes: size})
	}
	sort.Slice(largest, func(i, j int) bool { return largest[i].Bytes > largest[j].Bytes })
	if len(largest) > 5 {
		largest = largest[:5]
	}
	return untrackedStatsResult{Files: files, Bytes: bytes, Largest: largest}, nil
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
