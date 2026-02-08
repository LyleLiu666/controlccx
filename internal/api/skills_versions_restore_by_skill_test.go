package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/skills"
)

func TestAPI_SkillVersionsBySkill_Restore(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	sourceRoot := filepath.Join(home, ".agent", "skills")
	skillRoot := filepath.Join(sourceRoot, "skill-a")
	mustMkdirAll(t, skillRoot)
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v1\n")

	vers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 4, 10, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}
	if _, err := vers.Create(ctx, "skill-a", skills.CreateVersionInput{ID: "20260204-01"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Change current source to v2 and mount via Cursor copy-mode, so restore must resync managed copies.
	mustWriteFile(t, filepath.Join(skillRoot, "README.md"), "v2\n")

	skillsSvc, err := skills.NewService(skills.Options{HomeDir: home})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	if err := skillsSvc.Sync(ctx, "skill-a", skills.TargetCursor, false); err != nil {
		t.Fatalf("sync cursor: %v", err)
	}

	apiSvc := &API{Skills: skillsSvc, SkillVersionsBySkill: vers}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{"id": "20260204-01"})
	res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/restore", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}

	if got, _ := os.ReadFile(filepath.Join(skillRoot, "README.md")); string(got) != "v1\n" {
		t.Fatalf("source README=%q", string(got))
	}
	cursorCopy := filepath.Join(home, ".cursor", "skills", "skill-a", "README.md")
	if got, _ := os.ReadFile(cursorCopy); string(got) != "v1\n" {
		t.Fatalf("cursor README=%q", string(got))
	}

	backup := filepath.Join(home, ".agent", "skills_versions", "by_skill", "skill-a", "20260204-02", "README.md")
	if got, _ := os.ReadFile(backup); string(got) != "v2\n" {
		t.Fatalf("backup README=%q", string(got))
	}
}
