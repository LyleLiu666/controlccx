package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const versionsManifestFile = ".controlccx_skill_version.json"

type Version struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at,omitempty"`
	Note      string `json:"note,omitempty"`
}

type VersionsListResponse struct {
	SourceRoot   string    `json:"source_root"`
	VersionsRoot string    `json:"versions_root"`
	Versions     []Version `json:"versions"`
}

type VersionsOptions struct {
	HomeDir      string
	SourceRoot   string
	VersionsRoot string
	Now          func() time.Time
}

type VersionsService struct {
	homeDir      string
	sourceRoot   string
	versionsRoot string
	now          func() time.Time
}

func NewVersionsService(opts VersionsOptions) (*VersionsService, error) {
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
		sourceRoot = filepath.Join(home, ".agent", "skills")
	} else {
		sourceRoot = expandHome(sourceRoot, home)
		if !filepath.IsAbs(sourceRoot) {
			sourceRoot = filepath.Join(home, sourceRoot)
		}
		sourceRoot = filepath.Clean(sourceRoot)
	}

	versionsRoot := strings.TrimSpace(opts.VersionsRoot)
	if versionsRoot == "" {
		versionsRoot = filepath.Join(home, ".agent", "skills_versions")
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

	return &VersionsService{
		homeDir:      home,
		sourceRoot:   sourceRoot,
		versionsRoot: versionsRoot,
		now:          now,
	}, nil
}

func (s *VersionsService) List(ctx context.Context) (VersionsListResponse, error) {
	_ = ctx
	var versions []Version

	entries, err := os.ReadDir(s.versionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return VersionsListResponse{
				SourceRoot:   s.sourceRoot,
				VersionsRoot: s.versionsRoot,
				Versions:     nil,
			}, nil
		}
		return VersionsListResponse{}, fmt.Errorf("skills: read versions root %q: %w", s.versionsRoot, err)
	}

	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeVersionID(name) {
			continue
		}
		if !e.IsDir() {
			continue
		}
		v := Version{ID: name}
		if m, err := readVersionManifest(filepath.Join(s.versionsRoot, name)); err == nil {
			v.CreatedAt = m.CreatedAt
			v.Note = m.Note
		}
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool { return versions[i].ID > versions[j].ID })

	return VersionsListResponse{
		SourceRoot:   s.sourceRoot,
		VersionsRoot: s.versionsRoot,
		Versions:     versions,
	}, nil
}

type CreateVersionInput struct {
	ID   string `json:"id"`
	Note string `json:"note,omitempty"`
}

func (s *VersionsService) Create(ctx context.Context, input CreateVersionInput) (Version, error) {
	_ = ctx
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = s.nextAutoID()
	}
	if !isSafeVersionID(id) {
		return Version{}, fmt.Errorf("skills: invalid version id")
	}

	if err := os.MkdirAll(s.versionsRoot, 0o755); err != nil {
		return Version{}, fmt.Errorf("skills: ensure versions root %q: %w", s.versionsRoot, err)
	}

	finalPath := filepath.Join(s.versionsRoot, id)
	if _, err := os.Stat(finalPath); err == nil {
		return Version{}, fmt.Errorf("skills: version already exists: %s", id)
	} else if err != nil && !os.IsNotExist(err) {
		return Version{}, fmt.Errorf("skills: stat version: %w", err)
	}

	tmpDir, err := os.MkdirTemp(s.versionsRoot, ".tmp-skillver-")
	if err != nil {
		return Version{}, fmt.Errorf("skills: create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := s.copySourceTo(tmpDir); err != nil {
		return Version{}, err
	}

	createdAt := s.now().UTC().Format(time.RFC3339)
	if err := writeVersionManifest(tmpDir, versionManifest{
		ID:         id,
		CreatedAt:  createdAt,
		Note:       strings.TrimSpace(input.Note),
		SourceRoot: s.sourceRoot,
	}); err != nil {
		return Version{}, err
	}

	if err := os.Rename(tmpDir, finalPath); err != nil {
		return Version{}, fmt.Errorf("skills: finalize version: %w", err)
	}

	return Version{ID: id, CreatedAt: createdAt, Note: strings.TrimSpace(input.Note)}, nil
}

func (s *VersionsService) Delete(ctx context.Context, id string) error {
	_ = ctx
	id = strings.TrimSpace(id)
	if !isSafeVersionID(id) {
		return fmt.Errorf("skills: invalid version id")
	}
	path := filepath.Join(s.versionsRoot, id)
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

func (s *VersionsService) nextAutoID() string {
	day := s.now().Local().Format("20060102")
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("%s-%02d", day, i)
		if _, err := os.Stat(filepath.Join(s.versionsRoot, id)); os.IsNotExist(err) {
			return id
		}
	}
	// Extremely unlikely; fall back to timestamp.
	return fmt.Sprintf("%s-%d", day, s.now().Unix())
}

func (s *VersionsService) copySourceTo(dst string) error {
	dst = filepath.Clean(dst)

	entries, err := os.ReadDir(s.sourceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skills: read source root %q: %w", s.sourceRoot, err)
	}

	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeName(name) {
			continue
		}
		full := filepath.Join(s.sourceRoot, name)
		isLink := e.Type()&os.ModeSymlink != 0
		if !(e.IsDir() || isLink) {
			continue
		}

		dest := filepath.Join(dst, name)
		resolved := full
		if isLink {
			v, err := filepath.EvalSymlinks(full)
			if err != nil || strings.TrimSpace(v) == "" {
				continue
			}
			resolved = v
		}

		fi, err := os.Stat(resolved)
		if err != nil || !fi.IsDir() {
			continue
		}
		if err := copyDir(resolved, dest); err != nil {
			return fmt.Errorf("skills: snapshot %q: %w", name, err)
		}
	}
	return nil
}

type versionManifest struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"created_at"`
	Note       string `json:"note,omitempty"`
	SourceRoot string `json:"source_root"`
	Skill      string `json:"skill,omitempty"`
}

func writeVersionManifest(dir string, m versionManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: marshal version manifest: %w", err)
	}
	path := filepath.Join(filepath.Clean(dir), versionsManifestFile)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("skills: write version manifest: %w", err)
	}
	return nil
}

func readVersionManifest(dir string) (versionManifest, error) {
	path := filepath.Join(filepath.Clean(dir), versionsManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return versionManifest{}, err
	}
	var out versionManifest
	if err := json.Unmarshal(data, &out); err != nil {
		return versionManifest{}, err
	}
	return out, nil
}

func isSafeVersionID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return false
	}
	if strings.Contains(id, "..") {
		return false
	}
	return true
}
