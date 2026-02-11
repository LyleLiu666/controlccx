package skills

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Target string

const (
	TargetClaudeCode  Target = "claude_code"
	TargetCodex       Target = "codex"
	TargetCursor      Target = "cursor"
	TargetAntigravity Target = "antigravity"
	TargetOpencode    Target = "opencode"
)

type EntryStatus string

const (
	StatusMissing  EntryStatus = "missing"
	StatusLinked   EntryStatus = "linked"
	StatusBroken   EntryStatus = "broken"
	StatusPresent  EntryStatus = "present"
	StatusCopied   EntryStatus = "copied"
	StatusConflict EntryStatus = "conflict"
	StatusExternal EntryStatus = "external"
)

const managedMarkerFile = ".controlccx_skill_source"

type TargetRoot struct {
	Target Target `json:"target"`
	Root   string `json:"root"`
}

type TargetState struct {
	Target     Target      `json:"target"`
	Root       string      `json:"root"`
	Status     EntryStatus `json:"status"`
	LinkTarget string      `json:"link_target,omitempty"`
	Note       string      `json:"note,omitempty"`
}

type Skill struct {
	Name            string        `json:"name"`
	Sources         []string      `json:"sources,omitempty"`
	PreferredSource string        `json:"source,omitempty"`
	RepoKey         string        `json:"repo_key,omitempty"`
	RepoLabel       string        `json:"repo_label,omitempty"`
	RepoRef         string        `json:"repo_ref,omitempty"`
	Targets         []TargetState `json:"targets,omitempty"`

	// Optional per-skill snapshot status (filled by API layer when enabled).
	VersionsCount   int    `json:"versions_count,omitempty"`
	LatestVersionID string `json:"latest_version_id,omitempty"`
	NewVersion      bool   `json:"new_version,omitempty"`
	NewVersionAt    string `json:"new_version_at,omitempty"`
}

type RepoFacet struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Ref   string `json:"ref,omitempty"`
	Count int    `json:"count"`
}

type ListResponse struct {
	SourceRoots []string     `json:"source_roots"`
	Targets     []TargetRoot `json:"targets"`
	Skills      []Skill      `json:"skills"`
	Repos       []RepoFacet  `json:"repos,omitempty"`
}

type Options struct {
	HomeDir     string
	SourceRoots []string
	CodexHome   string
	Symlink     func(oldname, newname string) error
}

type Service struct {
	homeDir     string
	sourceRoots []string
	targetRoots map[Target][]string
	symlink     func(oldname, newname string) error
}

func NewService(opts Options) (*Service, error) {
	home := strings.TrimSpace(opts.HomeDir)
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(h) == "" {
			return nil, fmt.Errorf("skills: cannot determine home dir: %w", err)
		}
		home = h
	}
	home = filepath.Clean(home)

	sourceRoots := opts.SourceRoots
	if len(sourceRoots) == 0 {
		sourceRoots = []string{filepath.Join(home, ".agent", "skills")}
	}
	var normalizedRoots []string
	for _, r := range sourceRoots {
		p := expandHome(r, home)
		if !filepath.IsAbs(p) {
			p = filepath.Join(home, p)
		}
		p = filepath.Clean(p)
		normalizedRoots = append(normalizedRoots, p)
	}

	claudeRoot := filepath.Join(home, ".claude", "skills")
	cursorRoot := filepath.Join(home, ".cursor", "skills")
	antigravityRoot := filepath.Join(home, ".antigravity", "skills")
	opencodeRoot := filepath.Join(xdgConfigHome(home), "opencode", "skills")
	codexRoots := []string{filepath.Join(home, ".codex", "skills")}
	if strings.TrimSpace(opts.CodexHome) != "" {
		ch := expandHome(strings.TrimSpace(opts.CodexHome), home)
		if !filepath.IsAbs(ch) {
			ch = filepath.Join(home, ch)
		}
		ch = filepath.Clean(ch)
		codexRoots = append(codexRoots, filepath.Join(ch, "skills"))
	} else if ch := strings.TrimSpace(os.Getenv("CODEX_HOME")); ch != "" {
		ch = expandHome(ch, home)
		if !filepath.IsAbs(ch) {
			ch = filepath.Join(home, ch)
		}
		ch = filepath.Clean(ch)
		codexRoots = append(codexRoots, filepath.Join(ch, "skills"))
	}
	codexRoots = dedupePaths(codexRoots)

	symlinkFn := opts.Symlink
	if symlinkFn == nil {
		symlinkFn = os.Symlink
	}

	return &Service{
		homeDir:     home,
		sourceRoots: dedupePaths(normalizedRoots),
		targetRoots: map[Target][]string{
			TargetClaudeCode:  {claudeRoot},
			TargetCodex:       codexRoots,
			TargetCursor:      {cursorRoot},
			TargetAntigravity: {antigravityRoot},
			TargetOpencode:    {opencodeRoot},
		},
		symlink: symlinkFn,
	}, nil
}

func (s *Service) List(ctx context.Context) (ListResponse, error) {
	_ = ctx
	sourceByName := make(map[string][]string)
	allNames := make(map[string]bool)

	for _, root := range s.sourceRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ListResponse{}, fmt.Errorf("skills: read source root %q: %w", root, err)
		}
		for _, e := range entries {
			name := strings.TrimSpace(e.Name())
			if !isSafeName(name) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.IsDir() || (info.Mode()&os.ModeSymlink != 0) {
				allNames[name] = true
				sourceByName[name] = append(sourceByName[name], filepath.Join(root, name))
			}
		}
	}

	resolvedSourceRoots := s.resolveSourceRoots()
	targetEntries := make(map[Target]map[string]map[string]TargetState) // target->root->name->state
	for tgt, roots := range s.targetRoots {
		if targetEntries[tgt] == nil {
			targetEntries[tgt] = make(map[string]map[string]TargetState)
		}
		for _, root := range roots {
			m, err := scanTargetRoot(root, tgt, resolvedSourceRoots)
			if err != nil {
				return ListResponse{}, err
			}
			targetEntries[tgt][root] = m
			for name := range m {
				allNames[name] = true
			}
		}
	}

	var names []string
	for name := range allNames {
		names = append(names, name)
	}
	sort.Strings(names)

	var skills []Skill
	repoFacets := make(map[string]*RepoFacet)
	for _, name := range names {
		srcs := dedupePaths(sourceByName[name])
		preferred := ""
		if len(srcs) > 0 {
			preferred = srcs[0]
		}
		repoKey, repoLabel, repoRef := "", "", ""
		if preferred != "" {
			if m, err := readManagedManifest(preferred); err == nil {
				if md := deriveRepoMetadataFromManifest(m); md.Valid {
					repoKey = md.Key
					repoLabel = md.Label
					repoRef = md.Ref
				}
			}
		}
		var states []TargetState
		for tgt, roots := range s.targetRoots {
			for _, root := range roots {
				if st, ok := targetEntries[tgt][root][name]; ok {
					states = append(states, st)
				} else {
					states = append(states, TargetState{
						Target: tgt,
						Root:   root,
						Status: StatusMissing,
					})
				}
			}
		}
		skills = append(skills, Skill{
			Name:            name,
			Sources:         srcs,
			PreferredSource: preferred,
			RepoKey:         repoKey,
			RepoLabel:       repoLabel,
			RepoRef:         repoRef,
			Targets:         states,
		})
		if repoKey != "" {
			f, ok := repoFacets[repoKey]
			if !ok {
				f = &RepoFacet{Key: repoKey, Label: repoLabel, Ref: repoRef}
				repoFacets[repoKey] = f
			}
			if f.Label == "" {
				f.Label = repoLabel
			}
			if f.Ref == "" {
				f.Ref = repoRef
			}
			f.Count++
		}
	}

	var targets []TargetRoot
	for tgt, roots := range s.targetRoots {
		for _, root := range roots {
			targets = append(targets, TargetRoot{Target: tgt, Root: root})
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Target == targets[j].Target {
			return targets[i].Root < targets[j].Root
		}
		return targets[i].Target < targets[j].Target
	})

	repos := make([]RepoFacet, 0, len(repoFacets))
	for _, f := range repoFacets {
		repos = append(repos, *f)
	}
	sort.Slice(repos, func(i, j int) bool {
		li := strings.ToLower(strings.TrimSpace(repos[i].Label))
		lj := strings.ToLower(strings.TrimSpace(repos[j].Label))
		if li == lj {
			return repos[i].Key < repos[j].Key
		}
		return li < lj
	})

	return ListResponse{
		SourceRoots: s.sourceRoots,
		Targets:     targets,
		Skills:      skills,
		Repos:       repos,
	}, nil
}

func (s *Service) Link(ctx context.Context, name string, target Target) error {
	return s.Sync(ctx, name, target, false)
}

func (s *Service) Sync(ctx context.Context, name string, target Target, overwrite bool) error {
	_ = ctx
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return fmt.Errorf("skills: invalid skill name")
	}
	roots, ok := s.targetRoots[target]
	if !ok {
		return fmt.Errorf("skills: unknown target %q", target)
	}

	sourcePath, err := s.resolveSourcePath(name)
	if err != nil {
		return err
	}

	for _, root := range roots {
		forceCopy := target == TargetCursor
		if err := s.linkOne(target, root, name, sourcePath, overwrite, forceCopy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Unlink(ctx context.Context, name string, target Target) error {
	_ = ctx
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return fmt.Errorf("skills: invalid skill name")
	}
	roots, ok := s.targetRoots[target]
	if !ok {
		return fmt.Errorf("skills: unknown target %q", target)
	}
	for _, root := range roots {
		if err := s.unlinkOne(root, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) resolveSourcePath(name string) (string, error) {
	var candidates []string
	for _, root := range s.sourceRoots {
		p := filepath.Join(root, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("skills: source skill not found: %s", name)
	}
	// If multiple exist, pick the first (source root order), but keep it deterministic.
	return filepath.Clean(candidates[0]), nil
}

func (s *Service) resolveSourceRoots() []string {
	if s == nil {
		return nil
	}
	roots := make([]string, 0, len(s.sourceRoots)*2)
	for _, root := range s.sourceRoots {
		root = filepath.Clean(root)
		if root == "" {
			continue
		}
		roots = append(roots, root)
		if resolved, err := filepath.EvalSymlinks(root); err == nil && strings.TrimSpace(resolved) != "" {
			roots = append(roots, filepath.Clean(resolved))
		}
	}
	return dedupePaths(roots)
}

func (s *Service) linkOne(target Target, targetRoot, name, sourcePath string, overwrite bool, forceCopy bool) error {
	targetRoot = filepath.Clean(targetRoot)
	dest := filepath.Join(targetRoot, name)

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("skills: ensure target root %q: %w", targetRoot, err)
	}

	// Validate source is within allowed roots (resolved).
	resolvedSource, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return fmt.Errorf("skills: resolve source: %w", err)
	}
	resolvedSource = filepath.Clean(resolvedSource)
	resolvedRoots := s.resolveSourceRoots()
	if !isWithinAnyRoot(resolvedSource, resolvedRoots) {
		return fmt.Errorf("skills: source not under allowed roots: %s", resolvedSource)
	}

	// Handle existing dest.
	if st, err := os.Lstat(dest); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			if overwrite {
				if _, err := s.backupTargetEntry(target, name, dest); err != nil {
					return err
				}
			} else {
				// Only allow replacing a symlink if it points to allowed roots.
				linkTo, _ := os.Readlink(dest)
				if !isAllowedLinkTarget(targetRoot, linkTo, resolvedRoots) {
					return errTargetExists(dest)
				}
				_ = os.Remove(dest)
			}
		} else if st.IsDir() {
			if isManagedCopy(dest) {
				_ = os.RemoveAll(dest)
			} else if overwrite {
				if _, err := s.backupTargetEntry(target, name, dest); err != nil {
					return err
				}
			} else {
				return errTargetExists(dest)
			}
		} else {
			if overwrite {
				if _, err := s.backupTargetEntry(target, name, dest); err != nil {
					return err
				}
			} else {
				return errTargetExists(dest)
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("skills: stat target: %w", err)
	}

	if !forceCopy {
		linkTarget := sourcePath
		if rel, err := filepath.Rel(targetRoot, sourcePath); err == nil {
			linkTarget = rel
		}
		if err := s.symlink(linkTarget, dest); err == nil {
			return nil
		}
	}

	// Fallback: copy the directory and mark it as managed.
	if err := copyDir(resolvedSource, dest); err != nil {
		return fmt.Errorf("skills: copy fallback failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dest, managedMarkerFile), []byte(resolvedSource+"\n"), 0o644); err != nil {
		return fmt.Errorf("skills: write marker: %w", err)
	}
	return nil
}

func (s *Service) backupTargetEntry(target Target, name, entryPath string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("skills: service is nil")
	}
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return "", fmt.Errorf("skills: invalid skill name")
	}
	entryPath = filepath.Clean(strings.TrimSpace(entryPath))
	if entryPath == "" {
		return "", fmt.Errorf("skills: backup: empty entry path")
	}

	// Backups are stored outside any tool skills roots to avoid polluting scans.
	backupRoot := filepath.Join(s.homeDir, ".controlccx", "skills_backups", string(target), name)
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", fmt.Errorf("skills: backup: mkdir: %w", err)
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	backupPath := filepath.Join(backupRoot, stamp)
	for i := 0; i < 1000; i++ {
		if _, err := os.Lstat(backupPath); os.IsNotExist(err) {
			break
		}
		backupPath = filepath.Join(backupRoot, fmt.Sprintf("%s-%02d", stamp, i+1))
	}

	if err := os.Rename(entryPath, backupPath); err != nil {
		return "", fmt.Errorf("skills: backup: move %q -> %q: %w", entryPath, backupPath, err)
	}
	return backupPath, nil
}

func (s *Service) unlinkOne(targetRoot, name string) error {
	targetRoot = filepath.Clean(targetRoot)
	dest := filepath.Join(targetRoot, name)
	st, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skills: stat target: %w", err)
	}

	if st.Mode()&os.ModeSymlink != 0 {
		linkTo, err := os.Readlink(dest)
		if err != nil {
			return fmt.Errorf("skills: readlink: %w", err)
		}
		if !isAllowedLinkTarget(targetRoot, linkTo, s.resolveSourceRoots()) {
			return fmt.Errorf("skills: refuse to unlink unmanaged link: %s", dest)
		}
		return os.Remove(dest)
	}

	if st.IsDir() && isManagedCopy(dest) {
		return os.RemoveAll(dest)
	}
	return fmt.Errorf("skills: refuse to unlink non-link entry: %s", dest)
}

func scanTargetRoot(root string, target Target, allowedSourceRoots []string) (map[string]TargetState, error) {
	out := make(map[string]TargetState)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("skills: read target root %q: %w", root, err)
	}
	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeName(name) {
			continue
		}
		full := filepath.Join(root, name)
		st := TargetState{Target: target, Root: root, Status: StatusPresent}

		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTo, err := os.Readlink(full)
			if err != nil {
				st.Status = StatusConflict
				st.Note = "unreadable symlink"
				out[name] = st
				continue
			}
			st.LinkTarget = linkTo
			abs := linkAbs(root, linkTo)
			if _, err := os.Stat(abs); err != nil {
				if os.IsNotExist(err) {
					st.Status = StatusBroken
				} else {
					st.Status = StatusConflict
					st.Note = err.Error()
				}
			} else if isWithinAnyRoot(bestEffortEval(abs), allowedSourceRoots) {
				st.Status = StatusLinked
			} else {
				st.Status = StatusExternal
			}
			out[name] = st
			continue
		}
		if info.IsDir() && isManagedCopy(full) {
			st.Status = StatusCopied
		}
		out[name] = st
	}
	return out, nil
}

func isSafeName(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return true
}

func expandHome(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

func xdgConfigHome(home string) string {
	home = filepath.Clean(strings.TrimSpace(home))
	if home == "" {
		home = "."
	}
	if raw := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); raw != "" {
		p := expandHome(raw, home)
		if !filepath.IsAbs(p) {
			p = filepath.Join(home, p)
		}
		return filepath.Clean(p)
	}
	return filepath.Join(home, ".config")
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range paths {
		c := filepath.Clean(strings.TrimSpace(p))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func isWithinAnyRoot(path string, roots []string) bool {
	p := filepath.Clean(path)
	for _, root := range roots {
		r := filepath.Clean(root)
		if r == "" {
			continue
		}
		if p == r {
			return true
		}
		sep := string(os.PathSeparator)
		if strings.HasPrefix(p, r+sep) {
			return true
		}
	}
	return false
}

func isAllowedLinkTarget(targetRoot, linkTo string, allowedRoots []string) bool {
	abs := linkAbs(targetRoot, linkTo)
	return isWithinAnyRoot(bestEffortEval(abs), allowedRoots)
}

func linkAbs(targetRoot, linkTo string) string {
	if filepath.IsAbs(linkTo) {
		return filepath.Clean(linkTo)
	}
	return filepath.Clean(filepath.Join(targetRoot, linkTo))
}

func bestEffortEval(path string) string {
	if v, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(v)
	}
	return filepath.Clean(path)
}

func isManagedCopy(dest string) bool {
	_, err := os.Stat(filepath.Join(dest, managedMarkerFile))
	return err == nil
}

func copyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == "" || dst == "" {
		return fmt.Errorf("copyDir: empty path")
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			// Best-effort: try to preserve symlink.
			linkTo, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			_ = os.RemoveAll(target)
			return os.Symlink(linkTo, target)
		default:
			return copyFile(path, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
