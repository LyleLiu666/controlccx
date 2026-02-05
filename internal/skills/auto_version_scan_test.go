package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutoVersionScanner_Run_CreatesBaselineSnapshot(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "skill-a"))
	mustWrite(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "hello\n")

	skillsSvc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	perSkillVersions, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}

	now := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	scanner := NewAutoVersionScanner(skillsSvc, perSkillVersions, AutoVersionScanOptions{
		Now: func() time.Time { return now },
	})

	if err := scanner.Run(ctx, true); err != nil {
		t.Fatalf("run: %v", err)
	}

	root := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read versions root: %v", err)
	}
	var versionDirs []string
	for _, e := range entries {
		if e.IsDir() {
			versionDirs = append(versionDirs, e.Name())
		}
	}
	if len(versionDirs) != 1 || versionDirs[0] != "20260204-01" {
		t.Fatalf("versions=%v, want [20260204-01]", versionDirs)
	}
	if _, err := os.Stat(filepath.Join(root, skillNewVersionMarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("did not expect new-version marker to exist, err=%v", err)
	}
}

func TestAutoVersionScanner_EnsureSkill_RevisionChangeCreatesSnapshotAndMarker(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdir(t, skillRoot)
	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v1\n")

	mustWriteManagedManifestForTest(t, skillRoot, ManagedSkillManifest{
		SchemaVersion:  1,
		Name:           "skill-a",
		SourceType:     sourceTypeGit,
		SourceRef:      "https://github.com/acme/repo",
		SourceSubpath:  "skills/skill-a",
		SourceRevision: "rev-1",
		UpdatedAt:      time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		CreatedAt:      time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})

	skillsSvc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	perSkillVersions, err := NewPerSkillVersionsService(PerSkillVersionsOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}

	now := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	scanner := NewAutoVersionScanner(skillsSvc, perSkillVersions, AutoVersionScanOptions{
		Now: func() time.Time { return now },
	})

	if err := scanner.EnsureSkill(ctx, "skill-a", false); err != nil {
		t.Fatalf("ensure baseline: %v", err)
	}

	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v2\n")
	mustWriteManagedManifestForTest(t, skillRoot, ManagedSkillManifest{
		SchemaVersion:  1,
		Name:           "skill-a",
		SourceType:     sourceTypeGit,
		SourceRef:      "https://github.com/acme/repo",
		SourceSubpath:  "skills/skill-a",
		SourceRevision: "rev-2",
		UpdatedAt:      time.Date(2026, 2, 5, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		CreatedAt:      time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})

	now = time.Date(2026, 2, 5, 10, 0, 0, 0, time.UTC)
	if err := scanner.EnsureSkill(ctx, "skill-a", false); err != nil {
		t.Fatalf("ensure update: %v", err)
	}

	st, err := scanner.Status("skill-a")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.VersionsCount != 2 {
		t.Fatalf("versions_count=%d, want 2", st.VersionsCount)
	}
	if st.LatestVersionID != "20260205-01" {
		t.Fatalf("latest_version_id=%q, want 20260205-01", st.LatestVersionID)
	}
	if !st.NewVersion {
		t.Fatalf("new_version=false, want true")
	}

	markerPath := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a", skillNewVersionMarkerFile)
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected marker at %s: %v", markerPath, err)
	}
}

func TestAutoVersionScanner_Status_ExpiresMarkerAfterBadgeTTL(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdir(t, skillRoot)
	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v1\n")

	skillsSvc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	perSkillVersions, err := NewPerSkillVersionsService(PerSkillVersionsOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}

	now := time.Date(2026, 2, 5, 10, 0, 0, 0, time.UTC)
	scanner := NewAutoVersionScanner(skillsSvc, perSkillVersions, AutoVersionScanOptions{
		Now: func() time.Time { return now },
		// Keep TTL short for the test.
		BadgeTTL: 2 * time.Hour,
	})

	// Ensure marker exists.
	if err := os.MkdirAll(filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a"), 0o755); err != nil {
		t.Fatalf("mkdir marker root: %v", err)
	}
	markerRoot := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a")
	if err := writeNewVersionMarker(markerRoot, newVersionMarker{
		UpdatedAt: now.Format(time.RFC3339),
		VersionID: "20260205-01",
		Revision:  "rev-2",
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if st, err := scanner.Status("skill-a"); err != nil || !st.NewVersion {
		t.Fatalf("status before=%+v err=%v", st, err)
	}

	now = now.Add(3 * time.Hour)
	if st, err := scanner.Status("skill-a"); err != nil || st.NewVersion {
		t.Fatalf("status after=%+v err=%v", st, err)
	}
	if _, err := os.Stat(filepath.Join(markerRoot, skillNewVersionMarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("expected marker to be deleted, err=%v", err)
	}

	// EnsureSkill should still be safe even when marker is absent.
	if err := scanner.EnsureSkill(ctx, "skill-a", true); err != nil {
		t.Fatalf("ensure ack: %v", err)
	}
}

func TestAutoVersionScanner_Run_SyncsFromToolRootThenCreatesBaseline(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	// Only exists in a tool root.
	toolRoot := filepath.Join(home, ".claude", "skills")
	mustMkdir(t, filepath.Join(toolRoot, "skill-b"))
	mustWrite(t, filepath.Join(toolRoot, "skill-b", "SKILL.md"), "hello\n")

	skillsSvc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	perSkillVersions, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}
	scanner := NewAutoVersionScanner(skillsSvc, perSkillVersions, AutoVersionScanOptions{
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
		},
	})

	if err := scanner.Run(ctx, true); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Synced into canonical.
	if got := mustRead(t, filepath.Join(sourceRoot, "skill-b", "SKILL.md")); got != "hello\n" {
		t.Fatalf("synced SKILL.md=%q", got)
	}

	// Baseline snapshot created.
	root := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-b")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read versions root: %v", err)
	}
	var versionDirs []string
	for _, e := range entries {
		if e.IsDir() {
			versionDirs = append(versionDirs, e.Name())
		}
	}
	if len(versionDirs) != 1 || !strings.HasSuffix(versionDirs[0], "-01") || len(versionDirs[0]) != len("20060102-01") {
		t.Fatalf("versions=%v, want one auto baseline id like YYYYMMDD-01", versionDirs)
	}
}

func mustWriteManagedManifestForTest(t *testing.T, skillDir string, m ManagedSkillManifest) {
	t.Helper()
	m.SchemaVersion = 1
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(skillDir, managedManifestFile), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
