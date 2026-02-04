package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPerSkillVersionsService_ListCreateDelete(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdir(t, filepath.Join(sourceRoot, "skill-a"))
	mustWrite(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "a\n")
	if err := writeManagedManifest(filepath.Join(sourceRoot, "skill-a"), ManagedSkillManifest{
		Name:           "skill-a",
		SourceType:     "git",
		SourceRef:      "https://github.com/acme/repo",
		SourceSubpath:  "skills/skill-a",
		SourceRevision: "deadbeef",
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	svc, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 1, 30, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}

	before, err := svc.List(ctx, "skill-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if before.Skill != "skill-a" {
		t.Fatalf("skill=%q", before.Skill)
	}
	if before.Manifest == nil || before.Manifest.SourceType != "git" || before.Manifest.SourceRef != "https://github.com/acme/repo" {
		t.Fatalf("manifest=%+v", before.Manifest)
	}
	if len(before.Versions) != 0 {
		t.Fatalf("versions=%v", before.Versions)
	}

	v1, err := svc.Create(ctx, "skill-a", CreateVersionInput{ID: "20260130-01"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v1.ID != "20260130-01" {
		t.Fatalf("id=%q", v1.ID)
	}
	wantPath := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a", "20260130-01")
	if _, err := os.Stat(filepath.Join(wantPath, "README.md")); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}

	// Duplicate should fail.
	if _, err := svc.Create(ctx, "skill-a", CreateVersionInput{ID: "20260130-01"}); err == nil {
		t.Fatalf("expected duplicate create to fail")
	}

	// Auto-generate should pick next slot for the day (scoped per skill).
	v2, err := svc.Create(ctx, "skill-a", CreateVersionInput{})
	if err != nil {
		t.Fatalf("create auto: %v", err)
	}
	if v2.ID != "20260130-02" {
		t.Fatalf("auto id=%q", v2.ID)
	}

	out, err := svc.List(ctx, "skill-a")
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(out.Versions) != 2 {
		t.Fatalf("versions=%v", out.Versions)
	}

	if err := svc.Delete(ctx, "skill-a", "20260130-01"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted, err=%v", err)
	}
}

func TestPerSkillVersionsService_CreateFallsBackToTargetRoots(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	targetRoot := filepath.Join(home, ".claude", "skills")
	mustMkdir(t, filepath.Join(targetRoot, "skill-b"))
	mustWrite(t, filepath.Join(targetRoot, "skill-b", "README.md"), "b\n")

	svc, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 1, 30, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}

	out, err := svc.List(ctx, "skill-b")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if out.SkillSource != filepath.Join(targetRoot, "skill-b") {
		t.Fatalf("skill_source=%q", out.SkillSource)
	}

	v, err := svc.Create(ctx, "skill-b", CreateVersionInput{ID: "20260130-01"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.ID != "20260130-01" {
		t.Fatalf("id=%q", v.ID)
	}

	wantPath := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-b", "20260130-01")
	if _, err := os.Stat(filepath.Join(wantPath, "README.md")); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
}

func TestPerSkillVersionsService_Restore(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdir(t, skillRoot)
	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v1\n")
	if err := writeManagedManifest(skillRoot, ManagedSkillManifest{
		Name:       "skill-a",
		SourceType: "git",
		SourceRef:  "https://github.com/acme/repo",
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	svc, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}

	if _, err := svc.Create(ctx, "skill-a", CreateVersionInput{ID: "20260204-01", Note: "v1"}); err != nil {
		t.Fatalf("create v1: %v", err)
	}

	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v2\n")
	if _, err := svc.Create(ctx, "skill-a", CreateVersionInput{ID: "20260204-02", Note: "v2"}); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	res, err := svc.Restore(ctx, "skill-a", RestoreVersionInput{ID: "20260204-01"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.BackupID != "20260204-03" {
		t.Fatalf("backup_id=%q", res.BackupID)
	}
	if got := mustRead(t, filepath.Join(skillRoot, "README.md")); got != "v1\n" {
		t.Fatalf("restored README=%q", got)
	}
	if _, err := os.Stat(filepath.Join(skillRoot, versionsManifestFile)); !os.IsNotExist(err) {
		t.Fatalf("did not expect %s to be restored into skill dir, err=%v", versionsManifestFile, err)
	}

	m, err := readManagedManifest(skillRoot)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.SourceType != "git" || m.SourceRef != "https://github.com/acme/repo" {
		t.Fatalf("manifest source=%+v", m)
	}
	if strings.TrimSpace(m.ContentHash) == "" {
		t.Fatalf("expected content_hash to be set")
	}

	backupPath := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a", "20260204-03", "README.md")
	if got := mustRead(t, backupPath); got != "v2\n" {
		t.Fatalf("backup README=%q", got)
	}
}

func TestPerSkillVersionsService_RestoreCreatesSkillIfMissing(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdir(t, skillRoot)
	mustWrite(t, filepath.Join(skillRoot, "README.md"), "v1\n")
	if err := writeManagedManifest(skillRoot, ManagedSkillManifest{
		Name:       "skill-a",
		SourceType: "git",
		SourceRef:  "https://github.com/acme/repo",
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	svc, err := NewPerSkillVersionsService(PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}

	if _, err := svc.Create(ctx, "skill-a", CreateVersionInput{ID: "20260204-01", Note: "v1"}); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := os.RemoveAll(skillRoot); err != nil {
		t.Fatalf("remove skill: %v", err)
	}

	if _, err := svc.Restore(ctx, "skill-a", RestoreVersionInput{ID: "20260204-01"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := mustRead(t, filepath.Join(skillRoot, "README.md")); got != "v1\n" {
		t.Fatalf("restored README=%q", got)
	}
	if _, err := readManagedManifest(skillRoot); err != nil {
		t.Fatalf("expected manifest to exist after restore, err=%v", err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
