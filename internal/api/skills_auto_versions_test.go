package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"controlccx/internal/skills"
)

const managedManifestFileName = ".controlccx_skill.json"
const newVersionMarkerFileName = ".controlccx_skill_new_version.json"

func TestAPI_Skills_AutoVersions_StatusAndAck(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdirAll(t, skillRoot)
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v1\n")

	t1 := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 5, 10, 0, 0, 0, time.UTC)
	writeManagedManifestForAPITest(t, skillRoot, skills.ManagedSkillManifest{
		Name:           "skill-a",
		SourceType:     "git",
		SourceRef:      "https://github.com/acme/repo",
		SourceSubpath:  "skills/skill-a",
		SourceRevision: "rev-1",
		CreatedAt:      t1.Format(time.RFC3339),
		UpdatedAt:      t1.Format(time.RFC3339),
	})

	skillsSvc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}
	perSkillVers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}

	var now atomic.Int64
	now.Store(t1.UnixNano())
	scanner := skills.NewAutoVersionScanner(skillsSvc, perSkillVers, skills.AutoVersionScanOptions{
		Now: func() time.Time { return time.Unix(0, now.Load()).UTC() },
		// Keep list-trigger quiet while we assert.
		ThrottleTTL: 24 * time.Hour,
	})

	apiSvc := &API{
		Skills:               skillsSvc,
		SkillVersionsBySkill: perSkillVers,
		SkillAutoVersionScan: scanner,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// Baseline then update (creates marker).
	if err := scanner.EnsureSkill(ctx, "skill-a", false); err != nil {
		t.Fatalf("ensure baseline: %v", err)
	}
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v2\n")
	writeManagedManifestForAPITest(t, skillRoot, skills.ManagedSkillManifest{
		Name:           "skill-a",
		SourceType:     "git",
		SourceRef:      "https://github.com/acme/repo",
		SourceSubpath:  "skills/skill-a",
		SourceRevision: "rev-2",
		CreatedAt:      t1.Format(time.RFC3339),
		UpdatedAt:      t2.Format(time.RFC3339),
	})
	now.Store(t2.UnixNano())
	if err := scanner.EnsureSkill(ctx, "skill-a", false); err != nil {
		t.Fatalf("ensure update: %v", err)
	}

	{
		res, err := http.Get(srv.URL + "/api/skills")
		if err != nil {
			t.Fatalf("get skills: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		var out struct {
			Skills []skills.Skill `json:"skills"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Skills) != 1 || out.Skills[0].Name != "skill-a" {
			t.Fatalf("skills=%v", out.Skills)
		}
		got := out.Skills[0]
		if got.VersionsCount != 2 {
			t.Fatalf("versions_count=%d, want 2", got.VersionsCount)
		}
		if got.LatestVersionID == "" {
			t.Fatalf("latest_version_id is empty")
		}
		if !got.NewVersion {
			t.Fatalf("new_version=false, want true")
		}
		if got.NewVersionAt != t2.Format(time.RFC3339) {
			t.Fatalf("new_version_at=%q, want %q", got.NewVersionAt, t2.Format(time.RFC3339))
		}
	}

	// Opening versions panel acknowledges new-version and clears marker.
	{
		res, err := http.Get(srv.URL + "/api/skills/skill-a/versions")
		if err != nil {
			t.Fatalf("get versions: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		var out skills.PerSkillVersionsListResponse
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Skill != "skill-a" {
			t.Fatalf("skill=%q", out.Skill)
		}
		if len(out.Versions) != 2 {
			t.Fatalf("versions=%v, want 2", out.Versions)
		}
	}

	{
		res, err := http.Get(srv.URL + "/api/skills")
		if err != nil {
			t.Fatalf("get skills: %v", err)
		}
		defer res.Body.Close()
		var out struct {
			Skills []skills.Skill `json:"skills"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Skills) != 1 {
			t.Fatalf("skills=%v", out.Skills)
		}
		if out.Skills[0].NewVersion {
			t.Fatalf("new_version=true, want false after ack")
		}
	}
}

func TestAPI_Skills_AutoVersions_ListTriggerIsThrottled(t *testing.T) {
	home := t.TempDir()

	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdirAll(t, skillRoot)
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v1\n")

	t1 := time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Hour) // within throttle window
	writeManagedManifestForAPITest(t, skillRoot, skills.ManagedSkillManifest{
		Name:           "skill-a",
		SourceType:     "git",
		SourceRef:      "https://github.com/acme/repo",
		SourceRevision: "rev-1",
		CreatedAt:      t1.Format(time.RFC3339),
		UpdatedAt:      t1.Format(time.RFC3339),
	})

	skillsSvc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}
	perSkillVers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{HomeDir: home})
	if err != nil {
		t.Fatalf("new per-skill versions: %v", err)
	}

	var now atomic.Int64
	now.Store(t1.UnixNano())
	scanner := skills.NewAutoVersionScanner(skillsSvc, perSkillVers, skills.AutoVersionScanOptions{
		Now:         func() time.Time { return time.Unix(0, now.Load()).UTC() },
		ThrottleTTL: 24 * time.Hour,
	})

	apiSvc := &API{
		Skills:               skillsSvc,
		SkillVersionsBySkill: perSkillVers,
		SkillAutoVersionScan: scanner,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// First list triggers scan (baseline created async).
	{
		res, err := http.Get(srv.URL + "/api/skills")
		if err != nil {
			t.Fatalf("get skills: %v", err)
		}
		res.Body.Close()
	}

	versionsRoot := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a")
	waitFor(t, 2*time.Second, func() bool {
		entries, err := os.ReadDir(versionsRoot)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.IsDir() {
				return true
			}
		}
		return false
	})
	time.Sleep(50 * time.Millisecond)

	// Update source but stay within throttle window. Second list should NOT scan, so no new snapshot.
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v2\n")
	writeManagedManifestForAPITest(t, skillRoot, skills.ManagedSkillManifest{
		Name:           "skill-a",
		SourceType:     "git",
		SourceRef:      "https://github.com/acme/repo",
		SourceRevision: "rev-2",
		CreatedAt:      t1.Format(time.RFC3339),
		UpdatedAt:      t2.Format(time.RFC3339),
	})
	now.Store(t2.UnixNano())

	{
		res, err := http.Get(srv.URL + "/api/skills")
		if err != nil {
			t.Fatalf("get skills: %v", err)
		}
		res.Body.Close()
	}

	// Give any (unexpected) async scan a chance to run.
	time.Sleep(200 * time.Millisecond)

	entries, err := os.ReadDir(versionsRoot)
	if err != nil {
		t.Fatalf("read versions root: %v", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 1 {
		t.Fatalf("versions=%v, want exactly 1 (throttled)", dirs)
	}
	if _, err := os.Stat(filepath.Join(versionsRoot, newVersionMarkerFileName)); !os.IsNotExist(err) {
		t.Fatalf("did not expect marker to exist under throttle, err=%v", err)
	}
}

func writeManagedManifestForAPITest(t *testing.T, skillDir string, m skills.ManagedSkillManifest) {
	t.Helper()
	m.SchemaVersion = 1
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(skillDir, managedManifestFileName), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
