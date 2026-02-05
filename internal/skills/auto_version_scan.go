package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const skillNewVersionMarkerFile = ".controlccx_skill_new_version.json"

type SkillVersionStatus struct {
	VersionsCount   int    `json:"versions_count,omitempty"`
	LatestVersionID string `json:"latest_version_id,omitempty"`
	NewVersion      bool   `json:"new_version,omitempty"`
	NewVersionAt    string `json:"new_version_at,omitempty"`
}

type AutoVersionScanOptions struct {
	Now         func() time.Time
	ThrottleTTL time.Duration
	BadgeTTL    time.Duration
}

type AutoVersionScanner struct {
	skills   *Service
	versions *PerSkillVersionsService

	now         func() time.Time
	throttleTTL time.Duration
	badgeTTL    time.Duration

	mu      sync.Mutex
	running bool
	lastRun time.Time
}

func NewAutoVersionScanner(skillsSvc *Service, versionsSvc *PerSkillVersionsService, opts AutoVersionScanOptions) *AutoVersionScanner {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	throttle := opts.ThrottleTTL
	if throttle <= 0 {
		throttle = 10 * time.Minute
	}
	badge := opts.BadgeTTL
	if badge <= 0 {
		badge = 48 * time.Hour
	}
	return &AutoVersionScanner{
		skills:      skillsSvc,
		versions:    versionsSvc,
		now:         now,
		throttleTTL: throttle,
		badgeTTL:    badge,
	}
}

func (s *AutoVersionScanner) TriggerAsync(ctx context.Context, force bool) {
	if s == nil {
		return
	}
	if s.skills == nil || s.versions == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !s.start(force) {
		return
	}
	go func() {
		defer s.finish()
		_ = s.runScan(context.WithoutCancel(ctx))
	}()
}

func (s *AutoVersionScanner) Run(ctx context.Context, force bool) error {
	if s == nil || s.skills == nil || s.versions == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !s.start(force) {
		return nil
	}
	defer s.finish()
	return s.runScan(ctx)
}

func (s *AutoVersionScanner) start(force bool) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	now := s.now()
	if !force && !s.lastRun.IsZero() && now.Sub(s.lastRun) < s.throttleTTL {
		return false
	}
	s.running = true
	return true
}

func (s *AutoVersionScanner) finish() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.running = false
	s.lastRun = s.now()
	s.mu.Unlock()
}

func (s *AutoVersionScanner) runScan(ctx context.Context) error {
	list, err := s.skills.List(ctx)
	if err != nil {
		return err
	}

	for _, sk := range list.Skills {
		name := strings.TrimSpace(sk.Name)
		if !isSafeName(name) {
			continue
		}
		_, _ = s.ensureSkill(ctx, name, ensureOpts{CreateBaseline: true, DetectUpdates: true, AckViewed: false})
	}
	return nil
}

func (s *AutoVersionScanner) EnsureSkill(ctx context.Context, skill string, ackViewed bool) error {
	if s == nil || s.skills == nil || s.versions == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return fmt.Errorf("skills: invalid skill name")
	}
	_, err := s.ensureSkill(ctx, skill, ensureOpts{CreateBaseline: true, DetectUpdates: true, AckViewed: ackViewed})
	return err
}

func (s *AutoVersionScanner) Status(skill string) (SkillVersionStatus, error) {
	if s == nil || s.versions == nil {
		return SkillVersionStatus{}, nil
	}
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return SkillVersionStatus{}, fmt.Errorf("skills: invalid skill name")
	}

	root := filepath.Join(s.versions.versionsRoot, skill)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return SkillVersionStatus{}, nil
		}
		return SkillVersionStatus{}, fmt.Errorf("skills: read versions root: %w", err)
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := strings.TrimSpace(e.Name())
		if !isSafeVersionID(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	status := SkillVersionStatus{
		VersionsCount: len(ids),
	}
	if len(ids) > 0 {
		status.LatestVersionID = ids[0]
	}

	marker, ok, err := s.readNewVersionMarker(root)
	if err != nil {
		return status, nil
	}
	if ok {
		status.NewVersion = true
		status.NewVersionAt = marker.UpdatedAt
	}
	return status, nil
}

type newVersionMarker struct {
	UpdatedAt string `json:"updated_at"`
	VersionID string `json:"version_id,omitempty"`
	Revision  string `json:"revision,omitempty"`
}

func (s *AutoVersionScanner) readNewVersionMarker(skillVersionsRoot string) (newVersionMarker, bool, error) {
	path := filepath.Join(filepath.Clean(skillVersionsRoot), skillNewVersionMarkerFile)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newVersionMarker{}, false, nil
		}
		return newVersionMarker{}, false, err
	}
	var m newVersionMarker
	if err := json.Unmarshal(b, &m); err != nil {
		_ = os.Remove(path)
		return newVersionMarker{}, false, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(m.UpdatedAt))
	if err != nil {
		_ = os.Remove(path)
		return newVersionMarker{}, false, nil
	}
	if s != nil && s.badgeTTL > 0 {
		if s.now().Sub(t) > s.badgeTTL {
			_ = os.Remove(path)
			return newVersionMarker{}, false, nil
		}
	}
	return m, true, nil
}

func (s *AutoVersionScanner) clearNewVersionMarker(skill string) {
	if s == nil || s.versions == nil {
		return
	}
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return
	}
	root := filepath.Join(s.versions.versionsRoot, skill)
	_ = os.Remove(filepath.Join(root, skillNewVersionMarkerFile))
}

type ensureOpts struct {
	CreateBaseline bool
	DetectUpdates  bool
	AckViewed      bool
}

func (s *AutoVersionScanner) ensureSkill(ctx context.Context, skill string, opts ensureOpts) (SkillVersionStatus, error) {
	if s == nil || s.skills == nil || s.versions == nil {
		return SkillVersionStatus{}, nil
	}
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return SkillVersionStatus{}, fmt.Errorf("skills: invalid skill name")
	}

	if opts.AckViewed {
		s.clearNewVersionMarker(skill)
	}

	// Ensure skill exists in canonical store (~/.agent) when possible.
	src, err := s.skills.resolveSourcePath(skill)
	if err != nil {
		// Try syncing/importing from tool roots if there is a single deterministic variant.
		if err := s.syncToCanonical(ctx, skill); err == nil {
			src, _ = s.skills.resolveSourcePath(skill)
		}
	}

	// Only auto-create baseline for skills that are actually present in the canonical store.
	// If still missing, return status without side effects.
	if strings.TrimSpace(src) == "" {
		return s.Status(skill)
	}
	src = filepath.Clean(src)

	skillVersionsRoot := filepath.Join(s.versions.versionsRoot, skill)
	latestID, latestPath, latestManifest, ok, err := latestSnapshotManifest(skillVersionsRoot)
	if err != nil {
		return SkillVersionStatus{}, err
	}

	// Baseline.
	if !ok {
		if opts.CreateBaseline {
			meta, anchor, err := s.currentSnapshotMeta(ctx, skill, src)
			if err != nil {
				return SkillVersionStatus{}, err
			}
			_, _ = s.createAutoSnapshot(ctx, skill, src, meta, anchor, "baseline")
		}
		return s.Status(skill)
	}

	if !opts.DetectUpdates {
		return s.Status(skill)
	}

	meta, anchor, err := s.currentSnapshotMeta(ctx, skill, src)
	if err != nil {
		return SkillVersionStatus{}, err
	}

	changed, kind, err := snapshotChanged(ctx, src, meta, latestPath, latestManifest)
	if err != nil {
		return SkillVersionStatus{}, err
	}
	if !changed {
		return s.Status(skill)
	}

	if kind == snapshotChangeContent {
		anchor = s.now().UTC()
		meta.SourceUpdatedAt = anchor.Format(time.RFC3339)
	}

	v, err := s.createAutoSnapshot(ctx, skill, src, meta, anchor, "update")
	if err != nil {
		return SkillVersionStatus{}, err
	}

	// Mark as "new version" for the Skills list badge.
	if !opts.AckViewed {
		_ = writeNewVersionMarker(filepath.Join(s.versions.versionsRoot, skill), newVersionMarker{
			UpdatedAt: meta.SourceUpdatedAt,
			VersionID: v.ID,
			Revision:  meta.SourceRevision,
		})
	} else {
		// Viewing the panel acknowledges the update immediately.
		s.clearNewVersionMarker(skill)
	}

	_ = latestID // reserved for future
	return s.Status(skill)
}

func (s *AutoVersionScanner) syncToCanonical(ctx context.Context, skill string) error {
	if s == nil || s.skills == nil {
		return errors.New("skills: service not configured")
	}
	cand, err := s.skills.pickAutoImportCandidate(skill, TargetClaudeCode)
	if err != nil {
		// Not fatal for scanning; return as-is so callers can skip.
		return err
	}
	_, err = s.skills.ImportExisting(ctx, ImportExistingInput{
		SourcePath: cand.RealPath,
		Name:       skill,
		Tool:       string(cand.Tool),
		Overwrite:  false,
	})
	return err
}

func writeNewVersionMarker(skillVersionsRoot string, m newVersionMarker) error {
	skillVersionsRoot = filepath.Clean(skillVersionsRoot)
	if skillVersionsRoot == "" {
		return fmt.Errorf("skills: marker: invalid root")
	}
	if err := os.MkdirAll(skillVersionsRoot, 0o755); err != nil {
		return fmt.Errorf("skills: marker: mkdir: %w", err)
	}
	m.UpdatedAt = strings.TrimSpace(m.UpdatedAt)
	if m.UpdatedAt == "" {
		m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: marker: marshal: %w", err)
	}
	path := filepath.Join(skillVersionsRoot, skillNewVersionMarkerFile)
	return writeFileAtomic(path, append(b, '\n'), 0o644)
}

type snapshotMeta struct {
	SourceType      string
	SourceRef       string
	SourceRevision  string
	SourceUpdatedAt string
	ContentHash     string
}

func (s *AutoVersionScanner) currentSnapshotMeta(ctx context.Context, skill string, src string) (snapshotMeta, time.Time, error) {
	now := s.now().UTC()
	meta := snapshotMeta{
		SourceUpdatedAt: now.Format(time.RFC3339),
	}

	if m, err := readManagedManifest(src); err == nil {
		meta.SourceType = strings.TrimSpace(m.SourceType)
		meta.SourceRef = strings.TrimSpace(m.SourceRef)
		meta.SourceRevision = strings.TrimSpace(m.SourceRevision)
		if strings.TrimSpace(m.UpdatedAt) != "" {
			meta.SourceUpdatedAt = strings.TrimSpace(m.UpdatedAt)
		}
	}

	if strings.TrimSpace(meta.SourceRevision) == "" && isGitRepo(src) {
		if rev, updatedAt, ok := gitRepoSignature(ctx, src); ok {
			meta.SourceType = sourceTypeGit
			if strings.TrimSpace(meta.SourceRef) == "" {
				meta.SourceRef = src
			}
			meta.SourceRevision = rev
			if strings.TrimSpace(updatedAt) != "" {
				meta.SourceUpdatedAt = strings.TrimSpace(updatedAt)
			}
		}
	}

	anchor := now
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(meta.SourceUpdatedAt)); err == nil {
		anchor = t.UTC()
	}

	// Fallback to content hash when we don't have a revision signature.
	if strings.TrimSpace(meta.SourceRevision) == "" {
		fp, err := dirFingerprint(src)
		if err != nil {
			return snapshotMeta{}, anchor, err
		}
		meta.ContentHash = fp
	}
	return meta, anchor, nil
}

func latestSnapshotManifest(skillVersionsRoot string) (id string, path string, m versionManifest, ok bool, _ error) {
	entries, err := os.ReadDir(skillVersionsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", versionManifest{}, false, nil
		}
		return "", "", versionManifest{}, false, fmt.Errorf("skills: read per-skill versions root %q: %w", skillVersionsRoot, err)
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := strings.TrimSpace(e.Name())
		if !isSafeVersionID(name) {
			continue
		}
		ids = append(ids, name)
	}
	if len(ids) == 0 {
		return "", "", versionManifest{}, false, nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	id = ids[0]
	path = filepath.Join(skillVersionsRoot, id)
	m, err = readVersionManifest(path)
	if err != nil {
		return id, path, versionManifest{}, true, nil
	}
	return id, path, m, true, nil
}

type snapshotChangeKind int

const (
	snapshotUnchanged snapshotChangeKind = iota
	snapshotChangeRevision
	snapshotChangeContent
)

func snapshotChanged(ctx context.Context, src string, current snapshotMeta, prevPath string, prev versionManifest) (bool, snapshotChangeKind, error) {
	_ = ctx
	curRev := strings.TrimSpace(current.SourceRevision)
	prevRev := strings.TrimSpace(prev.SourceRevision)
	if curRev != "" && prevRev != "" {
		if curRev != prevRev {
			return true, snapshotChangeRevision, nil
		}
	}

	curHash := strings.TrimSpace(current.ContentHash)
	prevHash := strings.TrimSpace(prev.ContentHash)
	if curHash == "" {
		fp, err := dirFingerprint(src)
		if err != nil {
			return false, snapshotUnchanged, err
		}
		curHash = fp
	}
	if prevHash == "" {
		fp, err := dirFingerprintFiltered(prevPath, map[string]bool{versionsManifestFile: true})
		if err != nil {
			return false, snapshotUnchanged, err
		}
		prevHash = fp
	}
	if curHash == "" || prevHash == "" {
		return false, snapshotUnchanged, nil
	}
	if curHash != prevHash {
		return true, snapshotChangeContent, nil
	}
	return false, snapshotUnchanged, nil
}

func dirFingerprintFiltered(root string, ignore map[string]bool) (string, error) {
	if ignore == nil {
		ignore = map[string]bool{}
	}
	ignoreMerged := map[string]bool{}
	for k, v := range fingerprintIgnoreNames {
		ignoreMerged[k] = v
	}
	for k, v := range ignore {
		ignoreMerged[k] = v
	}

	root = filepath.Clean(root)
	if root == "" {
		return "", fmt.Errorf("skills: fingerprint: empty root")
	}

	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ignoreMerged[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		_, _ = hasher.Write([]byte(rel))
		_, _ = hasher.Write([]byte{'\n'})

		typ := d.Type()
		if typ.IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = hasher.Write(b)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *AutoVersionScanner) createAutoSnapshot(ctx context.Context, skill string, src string, meta snapshotMeta, anchor time.Time, kind string) (Version, error) {
	_ = ctx
	skill = strings.TrimSpace(skill)
	if !isSafeName(skill) {
		return Version{}, fmt.Errorf("skills: invalid skill name")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "baseline"
	}

	skillVersionsRoot := filepath.Join(s.versions.versionsRoot, skill)
	if err := os.MkdirAll(skillVersionsRoot, 0o755); err != nil {
		return Version{}, fmt.Errorf("skills: ensure per-skill versions root %q: %w", skillVersionsRoot, err)
	}

	day := anchor.Local().Format("20060102")
	if strings.TrimSpace(day) == "" {
		day = s.now().Local().Format("20060102")
	}

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		id := nextAutoIDForDay(skillVersionsRoot, day, s.now)
		if !isSafeVersionID(id) {
			return Version{}, fmt.Errorf("skills: invalid version id")
		}

		finalPath := filepath.Join(skillVersionsRoot, id)

		tmpDir, err := os.MkdirTemp(skillVersionsRoot, ".tmp-skillver-")
		if err != nil {
			return Version{}, fmt.Errorf("skills: create temp dir: %w", err)
		}

		if err := copyDirFiltered(src, tmpDir, fingerprintIgnoreNames); err != nil {
			_ = os.RemoveAll(tmpDir)
			return Version{}, fmt.Errorf("skills: snapshot %q: %w", skill, err)
		}

		contentHash, _ := dirFingerprint(tmpDir)
		if strings.TrimSpace(meta.ContentHash) == "" {
			meta.ContentHash = contentHash
		}

		createdAt := anchor.UTC().Format(time.RFC3339)
		vm := versionManifest{
			ID:              id,
			CreatedAt:       createdAt,
			Note:            "",
			SourceRoot:      s.versions.sourceRoot,
			Skill:           skill,
			ContentHash:     strings.TrimSpace(meta.ContentHash),
			SourceType:      strings.TrimSpace(meta.SourceType),
			SourceRef:       strings.TrimSpace(meta.SourceRef),
			SourceRevision:  strings.TrimSpace(meta.SourceRevision),
			SourceUpdatedAt: strings.TrimSpace(meta.SourceUpdatedAt),
			Auto:            true,
			AutoKind:        kind,
		}
		if err := writeVersionManifest(tmpDir, vm); err != nil {
			_ = os.RemoveAll(tmpDir)
			return Version{}, err
		}

		if err := os.Rename(tmpDir, finalPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			if _, statErr := os.Stat(finalPath); statErr == nil {
				lastErr = err
				continue
			}
			return Version{}, fmt.Errorf("skills: finalize version: %w", err)
		}

		return Version{ID: id, CreatedAt: createdAt, Note: ""}, nil
	}
	if lastErr != nil {
		return Version{}, fmt.Errorf("skills: finalize version: %w", lastErr)
	}
	return Version{}, fmt.Errorf("skills: finalize version: exhausted retries")
}

func nextAutoIDForDay(skillVersionsRoot string, day string, now func() time.Time) string {
	day = strings.TrimSpace(day)
	if len(day) != 8 {
		if now != nil {
			day = now().Local().Format("20060102")
		} else {
			day = time.Now().Local().Format("20060102")
		}
	}
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("%s-%02d", day, i)
		if _, err := os.Stat(filepath.Join(skillVersionsRoot, id)); os.IsNotExist(err) {
			return id
		}
	}
	if now != nil {
		return fmt.Sprintf("%s-%d", day, now().Unix())
	}
	return fmt.Sprintf("%s-%d", day, time.Now().Unix())
}

func isGitRepo(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(filepath.Clean(root), ".git"))
	return err == nil
}

func gitRepoSignature(ctx context.Context, repoRoot string) (revision string, updatedAt string, ok bool) {
	repoRoot = filepath.Clean(repoRoot)
	if strings.TrimSpace(repoRoot) == "" {
		return "", "", false
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", "", false
	}

	revCmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "HEAD")
	revOut, err := revCmd.CombinedOutput()
	if err != nil {
		return "", "", false
	}
	revision = strings.TrimSpace(string(revOut))
	if revision == "" {
		return "", "", false
	}

	// Prefer local "updated" signals that move with pull/fetch, so badges don't expire immediately for old commits.
	if gitDir, okDir := resolveGitDir(repoRoot); okDir {
		if ts, okTS := gitUpdatedAtFromGitDir(gitDir); okTS {
			return revision, ts, true
		}
	}

	return revision, time.Now().UTC().Format(time.RFC3339), true
}

func resolveGitDir(repoRoot string) (gitDir string, ok bool) {
	p := filepath.Join(filepath.Clean(repoRoot), ".git")
	fi, err := os.Stat(p)
	if err != nil {
		return "", false
	}
	if fi.IsDir() {
		return p, true
	}
	if !fi.Mode().IsRegular() {
		return "", true
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", true
	}
	s := strings.TrimSpace(string(b))
	const prefix = "gitdir:"
	if len(s) < len(prefix) || !strings.EqualFold(strings.TrimSpace(s[:len(prefix)]), prefix) {
		return "", true
	}
	rest := strings.TrimSpace(s[len(prefix):])
	if rest == "" {
		return "", true
	}
	if !filepath.IsAbs(rest) {
		rest = filepath.Join(repoRoot, rest)
	}
	rest = filepath.Clean(rest)
	if st, err := os.Stat(rest); err == nil && st.IsDir() {
		return rest, true
	}
	return "", true
}

func gitUpdatedAtFromGitDir(gitDir string) (string, bool) {
	gitDir = filepath.Clean(gitDir)
	if strings.TrimSpace(gitDir) == "" {
		return "", false
	}
	paths := []string{
		filepath.Join(gitDir, "FETCH_HEAD"),
		filepath.Join(gitDir, "ORIG_HEAD"),
		filepath.Join(gitDir, "logs", "HEAD"),
		filepath.Join(gitDir, "HEAD"),
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			return st.ModTime().UTC().Format(time.RFC3339), true
		}
	}
	return "", false
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".controlccx-skill-*")
	if err != nil {
		return fmt.Errorf("skills: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if runtime.GOOS != "windows" {
		_ = tmp.Chmod(perm)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("skills: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("skills: close temp: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("skills: rename temp: %w", err)
	}
	return nil
}
