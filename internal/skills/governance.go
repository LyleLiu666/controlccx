package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type ToolInfo struct {
	Key         string   `json:"key"`
	DisplayName string   `json:"display_name"`
	Installed   bool     `json:"installed"`
	DetectPaths []string `json:"detect_paths,omitempty"`
	SkillsRoots []string `json:"skills_roots,omitempty"`
}

func (s *Service) ListTools(ctx context.Context) ([]ToolInfo, error) {
	_ = ctx
	if s == nil {
		return nil, fmt.Errorf("skills: service is nil")
	}
	tools := []ToolInfo{
		{
			Key:         string(TargetCursor),
			DisplayName: "Cursor",
			SkillsRoots: dedupePaths(s.targetRoots[TargetCursor]),
		},
		{
			Key:         string(TargetClaudeCode),
			DisplayName: "Claude Code",
			SkillsRoots: dedupePaths(s.targetRoots[TargetClaudeCode]),
		},
		{
			Key:         string(TargetCodex),
			DisplayName: "Codex",
			SkillsRoots: dedupePaths(s.targetRoots[TargetCodex]),
		},
	}

	for i := range tools {
		var detects []string
		for _, r := range tools[i].SkillsRoots {
			parent := filepath.Clean(filepath.Dir(r))
			if parent != "" {
				detects = append(detects, parent)
			}
		}
		detects = dedupePaths(detects)
		installed := false
		for _, p := range detects {
			if _, err := os.Stat(p); err == nil {
				installed = true
				break
			}
		}
		tools[i].DetectPaths = detects
		tools[i].Installed = installed
	}

	return tools, nil
}

type OnboardingVariant struct {
	Tool        string `json:"tool"`
	Root        string `json:"root"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint,omitempty"`
	IsLink      bool   `json:"is_link,omitempty"`
	LinkTarget  string `json:"link_target,omitempty"`
}

type OnboardingGroup struct {
	Name        string              `json:"name"`
	Variants    []OnboardingVariant `json:"variants"`
	HasConflict bool                `json:"has_conflict"`
}

type OnboardingPlan struct {
	TotalToolsScanned int               `json:"total_tools_scanned"`
	TotalSkillsFound  int               `json:"total_skills_found"`
	Groups            []OnboardingGroup `json:"groups"`
}

func (s *Service) OnboardingPlan(ctx context.Context) (OnboardingPlan, error) {
	if s == nil {
		return OnboardingPlan{}, fmt.Errorf("skills: service is nil")
	}
	tools, err := s.ListTools(ctx)
	if err != nil {
		return OnboardingPlan{}, err
	}

	grouped := make(map[string][]OnboardingVariant)
	totalDetected := 0
	toolsScanned := 0

	for _, tool := range tools {
		if !tool.Installed {
			continue
		}
		toolsScanned++
		for _, root := range tool.SkillsRoots {
			variants, err := s.scanToolSkillsRoot(tool.Key, root)
			if err != nil {
				return OnboardingPlan{}, err
			}
			for _, v := range variants {
				totalDetected++
				grouped[v.Name] = append(grouped[v.Name], v)
			}
		}
	}

	var groups []OnboardingGroup
	for name, variants := range grouped {
		uniq := make(map[string]bool)
		for _, v := range variants {
			if v.Fingerprint != "" {
				uniq[v.Fingerprint] = true
			}
		}
		u := len(uniq)
		if u == 0 {
			u = 1
		}
		groups = append(groups, OnboardingGroup{
			Name:        name,
			Variants:    variants,
			HasConflict: u > 1,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })

	return OnboardingPlan{
		TotalToolsScanned: toolsScanned,
		TotalSkillsFound:  totalDetected,
		Groups:            groups,
	}, nil
}

func (s *Service) scanToolSkillsRoot(toolKey string, root string) ([]OnboardingVariant, error) {
	root = filepath.Clean(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: read tool root %q: %w", root, err)
	}

	var out []OnboardingVariant
	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeName(name) {
			continue
		}
		if toolKey == string(TargetCodex) && name == ".system" {
			continue
		}
		full := filepath.Join(root, name)

		info, err := e.Info()
		if err != nil {
			continue
		}

		isLink := info.Mode()&os.ModeSymlink != 0
		if !info.IsDir() && !isLink {
			continue
		}
		if isLink {
			// Only keep links that point to directories.
			if st, err := os.Stat(full); err != nil || !st.IsDir() {
				continue
			}
		}

		// Exclude already managed mounts.
		if info.IsDir() && isManagedCopy(full) {
			continue
		}
		if isLink {
			if resolved, err := filepath.EvalSymlinks(full); err == nil && isWithinAnyRoot(filepath.Clean(resolved), s.sourceRootsResolved) {
				continue
			}
		}

		linkTarget := ""
		if isLink {
			if v, err := os.Readlink(full); err == nil {
				linkTarget = v
			}
		}

		hashPath := full
		if isLink {
			if resolved, err := filepath.EvalSymlinks(full); err == nil && strings.TrimSpace(resolved) != "" {
				hashPath = resolved
			}
		}
		fp, _ := dirFingerprint(hashPath)

		out = append(out, OnboardingVariant{
			Tool:        toolKey,
			Root:        root,
			Name:        name,
			Path:        full,
			Fingerprint: fp,
			IsLink:      isLink,
			LinkTarget:  linkTarget,
		})
	}
	return out, nil
}

type ManagedSkill struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	SourceType     string `json:"source_type,omitempty"`
	SourceTool     string `json:"source_tool,omitempty"`
	SourceRef      string `json:"source_ref,omitempty"`
	SourceBranch   string `json:"source_branch,omitempty"`
	SourceSubpath  string `json:"source_subpath,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`
	ContentHash    string `json:"content_hash,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type InstallLocalInput struct {
	SourcePath string `json:"source_path"`
	Name       string `json:"name,omitempty"`
	Overwrite  bool   `json:"overwrite,omitempty"`
}

func (s *Service) InstallLocal(ctx context.Context, input InstallLocalInput) (ManagedSkill, error) {
	_ = ctx
	if s == nil {
		return ManagedSkill{}, fmt.Errorf("skills: service is nil")
	}
	sourcePath := filepath.Clean(strings.TrimSpace(input.SourcePath))
	if sourcePath == "" {
		return ManagedSkill{}, fmt.Errorf("skills: source path is required")
	}
	st, err := os.Stat(sourcePath)
	if err != nil || !st.IsDir() {
		return ManagedSkill{}, fmt.Errorf("skills: source path not found: %s", sourcePath)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(sourcePath)
	}
	if !isSafeName(name) {
		return ManagedSkill{}, fmt.Errorf("skills: invalid skill name")
	}

	root, err := s.canonicalRoot()
	if err != nil {
		return ManagedSkill{}, err
	}
	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		if !input.Overwrite {
			return ManagedSkill{}, errTargetExists(dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return ManagedSkill{}, fmt.Errorf("skills: overwrite remove dest: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return ManagedSkill{}, fmt.Errorf("skills: stat dest: %w", err)
	}

	tmp, err := os.MkdirTemp(root, ".tmp-controlccx-skill-install-")
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: create temp install dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := copyDirFiltered(sourcePath, tmp, fingerprintIgnoreNames); err != nil {
		return ManagedSkill{}, err
	}

	fp, _ := dirFingerprint(tmp)
	if err := writeManagedManifest(tmp, ManagedSkillManifest{
		Name:        name,
		SourceType:  sourceTypeLocal,
		SourceRef:   sourcePath,
		ContentHash: fp,
	}); err != nil {
		return ManagedSkill{}, err
	}
	fp, _ = dirFingerprint(tmp)

	if err := os.Rename(tmp, dest); err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: finalize install: %w", err)
	}

	m, _ := readManagedManifest(dest)
	return ManagedSkill{
		Name:        name,
		Path:        dest,
		SourceType:  m.SourceType,
		SourceTool:  m.SourceTool,
		SourceRef:   m.SourceRef,
		ContentHash: m.ContentHash,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}, nil
}

type ImportExistingInput struct {
	SourcePath string `json:"source_path"`
	Name       string `json:"name"`
	Tool       string `json:"tool,omitempty"`
	Overwrite  bool   `json:"overwrite,omitempty"`
}

func (s *Service) ImportExisting(ctx context.Context, input ImportExistingInput) (ManagedSkill, error) {
	_ = ctx
	if s == nil {
		return ManagedSkill{}, fmt.Errorf("skills: service is nil")
	}
	sourcePath := filepath.Clean(strings.TrimSpace(input.SourcePath))
	if sourcePath == "" {
		return ManagedSkill{}, fmt.Errorf("skills: source path is required")
	}
	st, err := os.Stat(sourcePath)
	if err != nil || !st.IsDir() {
		return ManagedSkill{}, fmt.Errorf("skills: source path not found: %s", sourcePath)
	}

	name := strings.TrimSpace(input.Name)
	if !isSafeName(name) {
		return ManagedSkill{}, fmt.Errorf("skills: invalid skill name")
	}

	root, err := s.canonicalRoot()
	if err != nil {
		return ManagedSkill{}, err
	}
	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		if !input.Overwrite {
			return ManagedSkill{}, errTargetExists(dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return ManagedSkill{}, fmt.Errorf("skills: overwrite remove dest: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return ManagedSkill{}, fmt.Errorf("skills: stat dest: %w", err)
	}

	tmp, err := os.MkdirTemp(root, ".tmp-controlccx-skill-install-")
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: create temp install dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := copyDirFiltered(sourcePath, tmp, fingerprintIgnoreNames); err != nil {
		return ManagedSkill{}, err
	}

	fp, _ := dirFingerprint(tmp)
	if err := writeManagedManifest(tmp, ManagedSkillManifest{
		Name:        name,
		SourceType:  sourceTypeImport,
		SourceTool:  strings.TrimSpace(input.Tool),
		SourceRef:   sourcePath,
		ContentHash: fp,
	}); err != nil {
		return ManagedSkill{}, err
	}
	fp, _ = dirFingerprint(tmp)

	if err := os.Rename(tmp, dest); err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: finalize import: %w", err)
	}

	m, _ := readManagedManifest(dest)
	return ManagedSkill{
		Name:        name,
		Path:        dest,
		SourceType:  m.SourceType,
		SourceTool:  m.SourceTool,
		SourceRef:   m.SourceRef,
		ContentHash: m.ContentHash,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}, nil
}

type GitSkillCandidate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Subpath     string `json:"subpath"`
}

func (s *Service) ListGitSkills(ctx context.Context, repoURL string) ([]GitSkillCandidate, error) {
	if s == nil {
		return nil, fmt.Errorf("skills: service is nil")
	}
	parsed := parseGitSource(repoURL)
	if strings.TrimSpace(parsed.cloneURL) == "" {
		return nil, fmt.Errorf("skills: repo url is required")
	}

	repoDir, _, err := cloneToTemp(ctx, parsed.cloneURL, parsed.branch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(repoDir) }()

	cands, err := listGitCandidates(repoDir, parsed.subpath)
	if err != nil {
		return nil, err
	}
	out := make([]GitSkillCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, GitSkillCandidate(c))
	}
	return out, nil
}

type InstallGitInput struct {
	RepoURL   string `json:"repo_url"`
	Subpath   string `json:"subpath,omitempty"`
	Name      string `json:"name,omitempty"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

func (s *Service) InstallGit(ctx context.Context, input InstallGitInput) (ManagedSkill, error) {
	if s == nil {
		return ManagedSkill{}, fmt.Errorf("skills: service is nil")
	}

	repoURL := strings.TrimSpace(input.RepoURL)
	if repoURL == "" {
		return ManagedSkill{}, fmt.Errorf("skills: repo url is required")
	}
	parsed := parseGitSource(repoURL)
	cloneURL := strings.TrimSpace(parsed.cloneURL)
	if cloneURL == "" {
		cloneURL = repoURL
	}

	subpath := strings.TrimSpace(input.Subpath)
	parsedSubpath := strings.TrimSpace(parsed.subpath)
	if subpath == "" {
		subpath = parsedSubpath
	}
	if subpath == "" {
		subpath = "."
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		if subpath != "." {
			name = filepath.Base(filepath.Clean(filepath.FromSlash(subpath)))
		} else {
			name = deriveNameFromRepoURL(cloneURL)
		}
	}
	if !isSafeName(name) {
		return ManagedSkill{}, fmt.Errorf("skills: invalid skill name")
	}

	root, err := s.canonicalRoot()
	if err != nil {
		return ManagedSkill{}, err
	}
	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		if !input.Overwrite {
			return ManagedSkill{}, errTargetExists(dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return ManagedSkill{}, fmt.Errorf("skills: overwrite remove dest: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return ManagedSkill{}, fmt.Errorf("skills: stat dest: %w", err)
	}

	repoDir, rev, err := cloneToTemp(ctx, cloneURL, parsed.branch)
	if err != nil {
		return ManagedSkill{}, err
	}
	defer func() { _ = os.RemoveAll(repoDir) }()

	if subpath == "." && strings.TrimSpace(input.Subpath) == "" && parsedSubpath == "" && isMultiSkillRepo(repoDir) {
		return ManagedSkill{}, errMultiSkills("该仓库包含多个 Skills，请先选择具体 Skill 文件夹")
	}

	copySrc, err := safeRepoSubpath(repoDir, subpath)
	if err != nil {
		return ManagedSkill{}, err
	}
	if st, err := os.Stat(copySrc); err != nil || !st.IsDir() {
		return ManagedSkill{}, fmt.Errorf("skills: path not found in repo: %s", subpath)
	}

	tmp, err := os.MkdirTemp(root, ".tmp-controlccx-skill-install-")
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: create temp install dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := copyDirFiltered(copySrc, tmp, fingerprintIgnoreNames); err != nil {
		return ManagedSkill{}, err
	}

	fp, _ := dirFingerprint(tmp)
	if err := writeManagedManifest(tmp, ManagedSkillManifest{
		Name:           name,
		SourceType:     sourceTypeGit,
		SourceRef:      repoURL,
		SourceBranch:   parsed.branch,
		SourceSubpath:  subpath,
		SourceRevision: rev,
		ContentHash:    fp,
	}); err != nil {
		return ManagedSkill{}, err
	}
	fp, _ = dirFingerprint(tmp)

	if err := os.Rename(tmp, dest); err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: finalize install: %w", err)
	}

	m, _ := readManagedManifest(dest)
	return ManagedSkill{
		Name:           name,
		Path:           dest,
		SourceType:     m.SourceType,
		SourceTool:     m.SourceTool,
		SourceRef:      m.SourceRef,
		SourceBranch:   m.SourceBranch,
		SourceSubpath:  m.SourceSubpath,
		SourceRevision: m.SourceRevision,
		ContentHash:    m.ContentHash,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}, nil
}

func (s *Service) UpdateManagedSkill(ctx context.Context, name string) (ManagedSkill, error) {
	if s == nil {
		return ManagedSkill{}, fmt.Errorf("skills: service is nil")
	}
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return ManagedSkill{}, fmt.Errorf("skills: invalid skill name")
	}
	root, err := s.canonicalRoot()
	if err != nil {
		return ManagedSkill{}, err
	}
	dest := filepath.Join(root, name)
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		return ManagedSkill{}, fmt.Errorf("skills: skill not found: %s", name)
	}

	old, err := readManagedManifest(dest)
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: not a managed skill: %s", name)
	}

	tmp, err := os.MkdirTemp(root, ".tmp-controlccx-skill-update-")
	if err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: create staging: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	next := old

	switch strings.TrimSpace(old.SourceType) {
	case sourceTypeLocal, sourceTypeImport:
		src := filepath.Clean(strings.TrimSpace(old.SourceRef))
		if src == "" {
			return ManagedSkill{}, fmt.Errorf("skills: missing source_ref for %s", old.SourceType)
		}
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			return ManagedSkill{}, fmt.Errorf("skills: source path not found: %s", src)
		}
		if err := copyDirFiltered(src, tmp, fingerprintIgnoreNames); err != nil {
			return ManagedSkill{}, err
		}
	case sourceTypeGit:
		srcURL := strings.TrimSpace(old.SourceRef)
		if srcURL == "" {
			return ManagedSkill{}, fmt.Errorf("skills: missing source_ref for git")
		}
		parsed := parseGitSource(srcURL)
		cloneURL := strings.TrimSpace(parsed.cloneURL)
		if cloneURL == "" {
			cloneURL = srcURL
		}
		branch := strings.TrimSpace(old.SourceBranch)
		if branch == "" {
			branch = parsed.branch
		}
		subpath := strings.TrimSpace(old.SourceSubpath)
		if subpath == "" {
			subpath = parsed.subpath
		}
		if subpath == "" {
			subpath = "."
		}

		repoDir, rev, err := cloneToTemp(ctx, cloneURL, branch)
		if err != nil {
			return ManagedSkill{}, err
		}
		defer func() { _ = os.RemoveAll(repoDir) }()

		copySrc, err := safeRepoSubpath(repoDir, subpath)
		if err != nil {
			return ManagedSkill{}, err
		}
		if st, err := os.Stat(copySrc); err != nil || !st.IsDir() {
			return ManagedSkill{}, fmt.Errorf("skills: path not found in repo: %s", subpath)
		}

		if err := copyDirFiltered(copySrc, tmp, fingerprintIgnoreNames); err != nil {
			return ManagedSkill{}, err
		}
		next.SourceBranch = branch
		next.SourceSubpath = subpath
		next.SourceRevision = rev
	default:
		return ManagedSkill{}, fmt.Errorf("skills: unsupported source_type: %s", old.SourceType)
	}

	fp, _ := dirFingerprint(tmp)
	next.ContentHash = fp
	next.Name = name
	next.CreatedAt = old.CreatedAt
	if err := writeManagedManifest(tmp, next); err != nil {
		return ManagedSkill{}, err
	}
	fp, _ = dirFingerprint(tmp)

	backup := filepath.Join(root, fmt.Sprintf(".controlccx-skill-backup-%s-%s", name, uuid.NewString()))
	if err := os.Rename(dest, backup); err != nil {
		return ManagedSkill{}, fmt.Errorf("skills: prepare swap: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Rename(backup, dest)
		return ManagedSkill{}, fmt.Errorf("skills: finalize swap: %w", err)
	}
	_ = os.RemoveAll(backup)

	// Refresh managed copy targets so copy-mode mounts follow updates.
	if err := s.resyncManagedCopies(name, dest); err != nil {
		return ManagedSkill{}, err
	}

	m, _ := readManagedManifest(dest)
	return ManagedSkill{
		Name:           name,
		Path:           dest,
		SourceType:     m.SourceType,
		SourceTool:     m.SourceTool,
		SourceRef:      m.SourceRef,
		SourceBranch:   m.SourceBranch,
		SourceSubpath:  m.SourceSubpath,
		SourceRevision: m.SourceRevision,
		ContentHash:    m.ContentHash,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}, nil
}

func (s *Service) canonicalRoot() (string, error) {
	if s == nil {
		return "", fmt.Errorf("skills: service is nil")
	}
	if len(s.sourceRoots) == 0 {
		return "", fmt.Errorf("skills: no source roots configured")
	}
	root := filepath.Clean(s.sourceRoots[0])
	if root == "" {
		return "", fmt.Errorf("skills: invalid source root")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("skills: ensure source root %q: %w", root, err)
	}
	return root, nil
}

func copyDirFiltered(src, dst string, ignore map[string]bool) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == "" || dst == "" {
		return fmt.Errorf("skills: copy: empty path")
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ignore != nil && ignore[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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

func (s *Service) resyncManagedCopies(name string, sourceDir string) error {
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return fmt.Errorf("skills: invalid skill name")
	}
	sourceDir = filepath.Clean(sourceDir)
	if sourceDir == "" {
		return fmt.Errorf("skills: invalid source dir")
	}

	for target, roots := range s.targetRoots {
		for _, root := range roots {
			dest := filepath.Join(filepath.Clean(root), name)
			st, err := os.Stat(dest)
			if err != nil || !st.IsDir() {
				continue
			}
			if !isManagedCopy(dest) {
				continue
			}
			_ = os.RemoveAll(dest)
			if err := copyDir(sourceDir, dest); err != nil {
				return fmt.Errorf("skills: resync %s %s: %w", target, dest, err)
			}
			if err := os.WriteFile(filepath.Join(dest, managedMarkerFile), []byte(sourceDir+"\n"), 0o644); err != nil {
				return fmt.Errorf("skills: resync marker: %w", err)
			}
		}
	}
	return nil
}
