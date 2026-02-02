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

func TestAPI_SkillVersions(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	// Seed a source root with one skill.
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-a"))
	mustWriteFile(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "a\n")

	vers, err := skills.NewVersionsService(skills.VersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new versions service: %v", err)
	}

	apiSvc := &API{SkillVersions: vers}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	t.Run("create", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/versions/create", "application/json", bytes.NewReader(buf))
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
		res, err := http.Post(srv.URL+"/api/skills/versions/create", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", res.StatusCode)
		}
	})

	t.Run("list", func(t *testing.T) {
		res, err := http.Get(srv.URL + "/api/skills/versions")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		var out skills.VersionsListResponse
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Versions) != 1 || out.Versions[0].ID != "20260130-01" {
			t.Fatalf("versions=%v", out.Versions)
		}
	})

	t.Run("delete", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/versions/delete", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		listRes, err := http.Get(srv.URL + "/api/skills/versions")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer listRes.Body.Close()
		var out skills.VersionsListResponse
		if err := json.NewDecoder(listRes.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Versions) != 0 {
			t.Fatalf("versions=%v", out.Versions)
		}
	})

	// Ensure create works even if context is unused (keeps signature consistent).
	if _, err := vers.Create(ctx, skills.CreateVersionInput{ID: "20260130-02"}); err != nil {
		t.Fatalf("create ctx: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
