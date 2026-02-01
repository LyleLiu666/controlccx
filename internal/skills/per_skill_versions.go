package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PerSkillVersionsListResponse struct {
	Skill       string    `json:"skill"`
	SourceRoot  string    `json:"source_root"`
	SkillSource string    `json:"skill_source"`
	VersionsRoot string   `json:"versions_root"`
	Versions    []Version `json:"versions"`
}

type PerSkillVersionsOptions struct {
	HomeDir      string
	SourceRoot   string
	VersionsRoot string
	Now          func() time.Time
}

type PerSkillVersionsService struct {
	homeDir      string
	sourceRoot   string
	versionsRoot string
	now          func() time.Time
}

func NewPerSkillVersionsService(opts PerSkillVersionsOptions) (*PerSkillVersionsService, error) {
	home := strings.TrimSpace(opts.HomeDir)
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(h) == "" {
			return nil, fmt.Errorf("skills: cannot determine home dir: %w", err)
		}
		home = h
	}
	home = filepath.Clean(home)

	sourceRoot := strings.TrimSpace(opts.SourceRoot)
	if sourceRoot == "" {
		sourceRoot = filepath.Join(home, ".agents", "skills")
	} else {
		sourceRoot = expandHome(sourceRoot, home)
		if !filepath.IsAbs(sourceRoot) {
			sourceRoot = filepath.Join(home, sourceRoot)
		}
		sourceRoot = filepath.Clean(sourceRoot)
	}

	versionsRoot := strings.TrimSpace(opts.VersionsRoot)
	if versionsRoot == "" {
		versionsRoot = filepath.Join(home, ".agents", "skills_versions", "by_skill")
	} else {
		versionsRoot = expandHome(versionsRoot, home)
		if !filepath.IsAbs(versionsRoot) {
			versionsRoot = filepath.Join(home, versionsRoot)
		}
		versionsRoot = filepath.Clean(versionsRoot)
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now() }
	}

	return &PerSkillVersionsService{
		homeDir:      home,
		sourceRoot:   sourceRoot,
		versionsRoot: versionsRoot,
		now:          now,
	}, nil
}

func (s *PerSkillVersionsService) List(ctx context.Context, skill string) (PerSkillVersionsListResponse, error) {
	_ = ctx
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return PerSkillVersionsListResponse{}, fmt.Errorf("skills: invalid skill name")
	}

	skillSource := filepath.Join(s.sourceRoot, skill)
	skillVersionsRoot := filepath.Join(s.versionsRoot, skill)

	entries, err := os.ReadDir(skillVersionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return PerSkillVersionsListResponse{
				Skill:        skill,
				SourceRoot:   s.sourceRoot,
				SkillSource:  skillSource,
				VersionsRoot: skillVersionsRoot,
				Versions:     nil,
			}, nil
		}
		return PerSkillVersionsListResponse{}, fmt.Errorf("skills: read per-skill versions root %q: %w", skillVersionsRoot, err)
	}

	var versions []Version
	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeVersionID(name) {
			continue
		}
		if !e.IsDir() {
			continue
		}
		v := Version{ID: name}
		if m, err := readVersionManifest(filepath.Join(skillVersionsRoot, name)); err == nil {
			v.CreatedAt = m.CreatedAt
			v.Note = m.Note
		}
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool { return versions[i].ID > versions[j].ID })

	return PerSkillVersionsListResponse{
		Skill:        skill,
		SourceRoot:   s.sourceRoot,
		SkillSource:  skillSource,
		VersionsRoot: skillVersionsRoot,
		Versions:     versions,
	}, nil
}

func (s *PerSkillVersionsService) Create(ctx context.Context, skill string, input CreateVersionInput) (Version, error) {
	_ = ctx
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return Version{}, fmt.Errorf("skills: invalid skill name")
	}

	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = s.nextAutoID(filepath.Join(s.versionsRoot, skill))
	}
	if !isSafeVersionID(id) {
		return Version{}, fmt.Errorf("skills: invalid version id")
	}

	skillVersionsRoot := filepath.Join(s.versionsRoot, skill)
	if err := os.MkdirAll(skillVersionsRoot, 0o755); err != nil {
		return Version{}, fmt.Errorf("skills: ensure per-skill versions root %q: %w", skillVersionsRoot, err)
	}

	finalPath := filepath.Join(skillVersionsRoot, id)
	if _, err := os.Stat(finalPath); err == nil {
		return Version{}, fmt.Errorf("skills: version already exists: %s", id)
	} else if err != nil && !os.IsNotExist(err) {
		return Version{}, fmt.Errorf("skills: stat version: %w", err)
	}

	tmpDir, err := os.MkdirTemp(skillVersionsRoot, ".tmp-skillver-")
	if err != nil {
		return Version{}, fmt.Errorf("skills: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	src, err := s.resolveSkillSource(skill)
	if err != nil {
		return Version{}, err
	}
	if err := copyDir(src, tmpDir); err != nil {
		return Version{}, fmt.Errorf("skills: snapshot %q: %w", skill, err)
	}

	createdAt := s.now().UTC().Format(time.RFC3339)
	if err := writeVersionManifest(tmpDir, versionManifest{
		ID:         id,
		CreatedAt:  createdAt,
		Note:       strings.TrimSpace(input.Note),
		SourceRoot: s.sourceRoot,
		Skill:      skill,
	}); err != nil {
		return Version{}, err
	}

	if err := os.Rename(tmpDir, finalPath); err != nil {
		return Version{}, fmt.Errorf("skills: finalize version: %w", err)
	}

	return Version{ID: id, CreatedAt: createdAt, Note: strings.TrimSpace(input.Note)}, nil
}

func (s *PerSkillVersionsService) Delete(ctx context.Context, skill, id string) error {
	_ = ctx
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return fmt.Errorf("skills: invalid skill name")
	}
	id = strings.TrimSpace(id)
	if !isSafeVersionID(id) {
		return fmt.Errorf("skills: invalid version id")
	}

	path := filepath.Join(s.versionsRoot, skill, id)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skills: stat version: %w", err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("skills: delete version: %w", err)
	}
	return nil
}

func (s *PerSkillVersionsService) nextAutoID(skillVersionsRoot string) string {
	day := s.now().Local().Format("20060102")
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("%s-%02d", day, i)
		if _, err := os.Stat(filepath.Join(skillVersionsRoot, id)); os.IsNotExist(err) {
			return id
		}
	}
	return fmt.Sprintf("%s-%d", day, s.now().Unix())
}

func (s *PerSkillVersionsService) resolveSkillSource(skill string) (string, error) {
	path := filepath.Join(s.sourceRoot, skill)
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("skills: skill not found: %s", skill)
		}
		return "", fmt.Errorf("skills: stat skill: %w", err)
	}

	resolved := path
	if fi.Mode()&os.ModeSymlink != 0 {
		v, err := filepath.EvalSymlinks(path)
		if err != nil || strings.TrimSpace(v) == "" {
			return "", fmt.Errorf("skills: unreadable skill symlink: %s", skill)
		}
		resolved = v
	}

	fi, err = os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("skills: stat skill: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("skills: skill source is not a directory: %s", skill)
	}
	return resolved, nil
}

