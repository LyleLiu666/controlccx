package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/skills"
)

func TestAPI_SkillVersionsBySkill(t *testing.T) {
	home := t.TempDir()

	sourceRoot := filepath.Join(home, ".agents", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-a"))
	mustWriteFile(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "a\n")

	vers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new per-skill versions service: %v", err)
	}

	apiSvc := &API{SkillVersionsBySkill: vers}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	t.Run("create", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/create", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		var out skills.Version
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.ID != "20260130-01" {
			t.Fatalf("id=%q", out.ID)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/create", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", res.StatusCode)
		}
	})

	t.Run("list", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/skills/skill-a/versions")
		if err != nil {
			t.Fatalf("get: %v", err)
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
		if len(out.Versions) != 1 || out.Versions[0].ID != "20260130-01" {
			t.Fatalf("versions=%v", out.Versions)
		}
	})

	t.Run("delete", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/delete", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}

		listRes, err := http.Get(srv.URL + "/api/skills/skill-a/versions")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer listRes.Body.Close()
		var out skills.PerSkillVersionsListResponse
		if err := json.NewDecoder(listRes.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Versions) != 0 {
			t.Fatalf("versions=%v", out.Versions)
		}
	})

	t.Run("create missing skill", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/missing/versions/create", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", res.StatusCode)
		}
	})
}

