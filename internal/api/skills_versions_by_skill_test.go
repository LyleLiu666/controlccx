package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/skills"
)

func TestAPI_SkillVersionsBySkill(t *testing.T) {
	home := t.TempDir()

	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-a"))
	mustWriteFile(t, filepath.Join(sourceRoot, "skill-a", "README.md"), "a\n")

	targetRoot := filepath.Join(home, ".claude", "skills")
	mustMkdirAll(t, filepath.Join(targetRoot, "skill-b"))
	mustWriteFile(t, filepath.Join(targetRoot, "skill-b", "README.md"), "b\n")

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

	t.Run("create from target root (no source)", func(t *testing.T) {
		body := map[string]any{"id": "20260130-01"}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/skills/skill-b/versions/create", "application/json", bytes.NewReader(buf))
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

func TestAPI_SkillVersionsBySkill_UpdateFromSource_AutoCreatesVersionWhenChanged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	repo := filepath.Join(home, "repo")
	mustMkdirAll(t, repo)
	mustGit(t, repo, "init")
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustMkdirAll(t, filepath.Join(repo, "skills", "skill-a"))
	mustWriteFile(t, filepath.Join(repo, "skills", "skill-a", "SKILL.md"), "v1\n")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "init")

	skillsSvc, err := skills.NewService(skills.Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	if _, err := skillsSvc.InstallGit(ctx, skills.InstallGitInput{
		RepoURL: repo,
		Subpath: "skills/skill-a",
		Name:    "skill-a",
	}); err != nil {
		t.Fatalf("install git skill: %v", err)
	}

	vers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 11, 12, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new versions service: %v", err)
	}

	// Simulate upstream update.
	mustWriteFile(t, filepath.Join(repo, "skills", "skill-a", "SKILL.md"), "v2\n")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "update skill-a")

	apiSvc := &API{Skills: skillsSvc, SkillVersionsBySkill: vers}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/update", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("post update: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}

	var out struct {
		OK      bool            `json:"ok"`
		Updated bool            `json:"updated"`
		Version *skills.Version `json:"version,omitempty"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK {
		t.Fatalf("ok=false")
	}
	if !out.Updated {
		t.Fatalf("updated=false want true")
	}
	if out.Version == nil || strings.TrimSpace(out.Version.ID) == "" {
		t.Fatalf("expected version to be created, got=%+v", out.Version)
	}

	got, err := os.ReadFile(filepath.Join(sourceRoot, "skill-a", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill source: %v", err)
	}
	if string(got) != "v2\n" {
		t.Fatalf("source content=%q want %q", string(got), "v2\n")
	}

	list, err := vers.List(ctx, "skill-a")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(list.Versions) != 1 {
		t.Fatalf("versions=%v", list.Versions)
	}
}

func TestAPI_SkillVersionsBySkill_UpdateFromSource_NoChange_NoVersionCreated(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	repo := filepath.Join(home, "repo")
	mustMkdirAll(t, repo)
	mustGit(t, repo, "init")
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustMkdirAll(t, filepath.Join(repo, "skills", "skill-a"))
	mustWriteFile(t, filepath.Join(repo, "skills", "skill-a", "SKILL.md"), "v1\n")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "init")

	skillsSvc, err := skills.NewService(skills.Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	if _, err := skillsSvc.InstallGit(ctx, skills.InstallGitInput{
		RepoURL: repo,
		Subpath: "skills/skill-a",
		Name:    "skill-a",
	}); err != nil {
		t.Fatalf("install git skill: %v", err)
	}

	vers, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{
		HomeDir: home,
		Now: func() time.Time {
			return time.Date(2026, 2, 11, 12, 0, 0, 0, time.Local)
		},
	})
	if err != nil {
		t.Fatalf("new versions service: %v", err)
	}

	apiSvc := &API{Skills: skillsSvc, SkillVersionsBySkill: vers}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/skills/skill-a/versions/update", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("post update: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}

	var out struct {
		OK      bool            `json:"ok"`
		Updated bool            `json:"updated"`
		Version *skills.Version `json:"version,omitempty"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK {
		t.Fatalf("ok=false")
	}
	if out.Updated {
		t.Fatalf("updated=true want false")
	}
	if out.Version != nil {
		t.Fatalf("expected no version, got=%+v", out.Version)
	}

	list, err := vers.List(ctx, "skill-a")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(list.Versions) != 0 {
		t.Fatalf("versions=%v", list.Versions)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
