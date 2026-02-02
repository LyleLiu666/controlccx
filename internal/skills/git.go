package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type gitSource struct {
	cloneURL string
	branch   string
	subpath  string
}

func parseGitSource(input string) gitSource {
	trimmed := strings.TrimSpace(strings.TrimSuffix(input, "/"))
	if trimmed == "" {
		return gitSource{}
	}

	// Accept GitHub shorthand like `owner/repo` (and `owner/repo/tree/<branch>/...`).
	normalized := trimmed
	if strings.HasPrefix(trimmed, "https://github.com/") {
		normalized = trimmed
	} else if strings.HasPrefix(trimmed, "http://github.com/") {
		normalized = strings.Replace(trimmed, "http://github.com/", "https://github.com/", 1)
	} else if strings.HasPrefix(trimmed, "github.com/") {
		normalized = "https://" + trimmed
	} else if looksLikeGitHubShorthand(trimmed) {
		normalized = "https://github.com/" + trimmed
	}

	const ghPrefix = "https://github.com/"
	if !strings.HasPrefix(normalized, ghPrefix) {
		return gitSource{cloneURL: normalized}
	}

	rest := strings.TrimPrefix(normalized, ghPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return gitSource{cloneURL: normalized}
	}

	owner := parts[0]
	repo := parts[1]
	repo = strings.TrimSuffix(repo, ".git")

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		branch := parts[3]
		subpath := ""
		if len(parts) > 4 {
			subpath = strings.Join(parts[4:], "/")
		}
		return gitSource{cloneURL: cloneURL, branch: branch, subpath: subpath}
	}

	return gitSource{cloneURL: cloneURL}
}

func looksLikeGitHubShorthand(input string) bool {
	if input == "" {
		return false
	}
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "~") || strings.HasPrefix(input, ".") {
		return false
	}
	if strings.Contains(input, "://") || strings.Contains(input, "@") || strings.Contains(input, ":") {
		return false
	}
	parts := strings.Split(input, "/")
	if len(parts) < 2 {
		return false
	}
	owner := parts[0]
	repo := parts[1]
	if owner == "" || repo == "" || owner == "." || owner == ".." || repo == "." || repo == ".." {
		return false
	}
	isSafeSeg := func(s string) bool {
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return false
		}
		return true
	}
	if !isSafeSeg(owner) || !isSafeSeg(strings.TrimSuffix(repo, ".git")) {
		return false
	}
	if len(parts) > 2 {
		return parts[2] == "tree" || parts[2] == "blob"
	}
	return true
}

func cloneToTemp(ctx context.Context, cloneURL, branch string) (repoDir string, revision string, err error) {
	cloneURL = strings.TrimSpace(cloneURL)
	branch = strings.TrimSpace(branch)
	if cloneURL == "" {
		return "", "", fmt.Errorf("skills: git clone: empty url")
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "", "", fmt.Errorf("skills: git not found in PATH")
	}

	tmp, err := os.MkdirTemp("", ".controlccx-skills-git-")
	if err != nil {
		return "", "", fmt.Errorf("skills: create temp dir: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmp)
		}
	}()

	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, cloneURL, tmp)

	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("skills: git clone failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	revCmd := exec.CommandContext(ctx, "git", "-C", tmp, "rev-parse", "HEAD")
	revOut, err := revCmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("skills: git rev-parse failed: %v (%s)", err, strings.TrimSpace(string(revOut)))
	}

	return tmp, strings.TrimSpace(string(revOut)), nil
}

func deriveNameFromRepoURL(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "skill"
	}
	repoURL = strings.TrimSuffix(repoURL, "/")

	if runtime.GOOS == "windows" {
		// Allow local paths like C:\x\y
		repoURL = strings.ReplaceAll(repoURL, "\\", "/")
	}

	base := repoURL
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	if base == "" {
		return "skill"
	}
	return base
}

func parseSkillFrontMatter(skillMDPath string) (name string, desc string, ok bool) {
	b, err := os.ReadFile(skillMDPath)
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"`))
		} else if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "description:")), `"`))
		}
	}
	if strings.TrimSpace(name) == "" {
		return "", "", false
	}
	return name, strings.TrimSpace(desc), true
}

func safeRepoSubpath(repoDir string, subpath string) (string, error) {
	repoDir = filepath.Clean(repoDir)
	subpath = strings.TrimSpace(subpath)
	if repoDir == "" {
		return "", fmt.Errorf("skills: invalid repo dir")
	}
	if subpath == "" || subpath == "." {
		return repoDir, nil
	}

	subpath = strings.ReplaceAll(subpath, "\\", "/")
	cleaned := filepath.Clean(filepath.FromSlash(subpath))
	if cleaned == "" || cleaned == "." {
		return repoDir, nil
	}
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("skills: invalid subpath")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("skills: invalid subpath")
	}

	joined := filepath.Clean(filepath.Join(repoDir, cleaned))
	if !isWithinAnyRoot(joined, []string{repoDir}) {
		return "", fmt.Errorf("skills: invalid subpath")
	}
	return joined, nil
}

func listGitCandidates(repoDir string, parsedSubpath string) ([]GitSkillCandidate, error) {
	repoDir = filepath.Clean(repoDir)
	if repoDir == "" {
		return nil, fmt.Errorf("skills: list git candidates: empty repo dir")
	}

	scanRoot, err := safeRepoSubpath(repoDir, parsedSubpath)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(scanRoot); err != nil || !fi.IsDir() {
		return nil, nil
	}

	ignoreDirs := map[string]bool{
		".git":         true,
		".hg":          true,
		".svn":         true,
		".ccx":         true,
		"node_modules": true,
		"__pycache__":  true,
		".venv":        true,
		"venv":         true,
	}

	var out []GitSkillCandidate
	if err := filepath.WalkDir(scanRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ignoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		dir := filepath.Dir(p)
		rel, err := filepath.Rel(repoDir, dir)
		if err != nil {
			return nil
		}
		subpath := filepath.ToSlash(rel)
		if subpath == "" {
			subpath = "."
		}
		subpath = strings.TrimPrefix(subpath, "./")
		if subpath == "" {
			subpath = "."
		}

		name := filepath.Base(dir)
		if subpath == "." {
			name = "root-skill"
		}
		desc := ""
		if n, d, ok := parseSkillFrontMatter(p); ok {
			name, desc = n, d
		}
		out = append(out, GitSkillCandidate{
			Name:        name,
			Description: desc,
			Subpath:     subpath,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	out = filterOutNestedGitCandidates(out)
	sortGitCandidates(out)
	out = dedupeGitCandidates(out)
	return out, nil
}

func cleanCandidateSubpath(subpath string) string {
	s := strings.TrimSpace(subpath)
	if s == "" || s == "." {
		return "."
	}
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimPrefix(s, "./")
	s = path.Clean(s)
	if s == "." {
		return "."
	}
	return s
}

func isCandidateNested(child, parent string) bool {
	child = cleanCandidateSubpath(child)
	parent = cleanCandidateSubpath(parent)
	if child == "." || child == parent {
		return false
	}
	if parent == "." {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}

func filterOutNestedGitCandidates(cands []GitSkillCandidate) []GitSkillCandidate {
	if len(cands) == 0 {
		return cands
	}

	type item struct {
		c     GitSkillCandidate
		path  string
		depth int
	}
	items := make([]item, 0, len(cands))
	for _, c := range cands {
		p := cleanCandidateSubpath(c.Subpath)
		if p == "" {
			p = "."
		}
		depth := 0
		if p != "." {
			depth = len(strings.Split(p, "/"))
		}
		c.Subpath = p
		items = append(items, item{c: c, path: p, depth: depth})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].depth == items[j].depth {
			return items[i].path < items[j].path
		}
		return items[i].depth < items[j].depth
	})

	var kept []GitSkillCandidate
	var keptPaths []string
	for _, it := range items {
		nested := false
		for _, p := range keptPaths {
			if isCandidateNested(it.path, p) {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		kept = append(kept, it.c)
		keptPaths = append(keptPaths, it.path)
	}
	return kept
}

func isMultiSkillRepo(repoDir string) bool {
	skillsDir := filepath.Join(filepath.Clean(repoDir), "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return false
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsDir, e.Name(), "SKILL.md")); err == nil {
			count++
		}
	}
	return count >= 2
}

func sortGitCandidates(cands []GitSkillCandidate) {
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Name == cands[j].Name {
			return cands[i].Subpath < cands[j].Subpath
		}
		return cands[i].Name < cands[j].Name
	})
}

func dedupeGitCandidates(cands []GitSkillCandidate) []GitSkillCandidate {
	seen := make(map[string]bool, len(cands))
	out := make([]GitSkillCandidate, 0, len(cands))
	for _, c := range cands {
		key := strings.TrimSpace(c.Subpath)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}
