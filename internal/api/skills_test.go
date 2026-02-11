package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/skills"
)

func TestAPI_Skills_ListLinkUnlink(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-creator"))

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/skills")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var list skills.ListResponse
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Skills) != 1 || list.Skills[0].Name != "skill-creator" {
		t.Fatalf("skills=%v", list.Skills)
	}

	body, _ := json.Marshal(map[string]string{"name": "skill-creator", "target": "claude"})
	linkRes, err := http.Post(srv.URL+"/api/skills/link", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post link: %v", err)
	}
	defer linkRes.Body.Close()
	if linkRes.StatusCode != http.StatusOK {
		t.Fatalf("link status=%d, want 200", linkRes.StatusCode)
	}

	dest := filepath.Join(home, ".claude", "skills", "skill-creator")
	if _, err := filepath.EvalSymlinks(dest); err != nil {
		t.Fatalf("expected linked entry, eval: %v", err)
	}

	unlinkBody, _ := json.Marshal(map[string]string{"name": "skill-creator", "target": "claude"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/skills/unlink", bytes.NewReader(unlinkBody))
	req.Header.Set("Content-Type", "application/json")
	unlinkRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post unlink: %v", err)
	}
	defer unlinkRes.Body.Close()
	if unlinkRes.StatusCode != http.StatusOK {
		t.Fatalf("unlink status=%d, want 200", unlinkRes.StatusCode)
	}
}

func TestAPI_Skills_ListPagingAndFilter(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-creator"))
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-one"))
	mustMkdirAll(t, filepath.Join(sourceRoot, "skill-two"))

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	type page struct {
		Skills []skills.Skill `json:"skills"`
		Total  int            `json:"total"`
		Offset int            `json:"offset"`
		Limit  int            `json:"limit"`
	}

	{
		res, err := http.Get(srv.URL + "/api/skills?limit=2&offset=0")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var got page
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Total != 3 {
			t.Fatalf("total=%d, want 3", got.Total)
		}
		if got.Limit != 2 || got.Offset != 0 {
			t.Fatalf("limit=%d offset=%d, want limit=2 offset=0", got.Limit, got.Offset)
		}
		if len(got.Skills) != 2 {
			t.Fatalf("skills=%d, want 2", len(got.Skills))
		}
	}

	{
		res, err := http.Get(srv.URL + "/api/skills?limit=2&offset=2")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var got page
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Total != 3 {
			t.Fatalf("total=%d, want 3", got.Total)
		}
		if got.Limit != 2 || got.Offset != 2 {
			t.Fatalf("limit=%d offset=%d, want limit=2 offset=2", got.Limit, got.Offset)
		}
		if len(got.Skills) != 1 {
			t.Fatalf("skills=%d, want 1", len(got.Skills))
		}
		if got.Skills[0].Name != "skill-two" {
			t.Fatalf("skill[0].name=%q, want skill-two", got.Skills[0].Name)
		}
	}

	{
		res, err := http.Get(srv.URL + "/api/skills?q=creator")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var got page
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Total != 1 {
			t.Fatalf("total=%d, want 1", got.Total)
		}
		if len(got.Skills) != 1 || got.Skills[0].Name != "skill-creator" {
			t.Fatalf("skills=%v, want only skill-creator", got.Skills)
		}
	}
}

func TestAPI_Skills_LinkAutoImportBootstrap(t *testing.T) {
	t.Setenv("CODEX_HOME", "")

	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	// Existing unmanaged variant in a tool dir.
	variant := filepath.Join(home, ".claude", "skills", "skill-x")
	mustMkdirAll(t, variant)
	if err := os.WriteFile(filepath.Join(variant, "SKILL.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write variant: %v", err)
	}

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"name":        "skill-x",
		"target":      "codex",
		"auto_import": true,
	})
	res, err := http.Post(srv.URL+"/api/skills/link", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post link: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d, want 200, body=%s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	// Source bootstrapped.
	gotSource, err := os.ReadFile(filepath.Join(sourceRoot, "skill-x", "SKILL.md"))
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(gotSource) != "hello\n" {
		t.Fatalf("source=%q, want %q", string(gotSource), "hello\\n")
	}

	// Target enabled (symlink or copy).
	gotTarget, err := os.ReadFile(filepath.Join(home, ".codex", "skills", "skill-x", "SKILL.md"))
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(gotTarget) != "hello\n" {
		t.Fatalf("target=%q, want %q", string(gotTarget), "hello\\n")
	}
}

func TestAPI_Skills_LinkAutoImportRefusesMultiVariants(t *testing.T) {
	t.Setenv("CODEX_HOME", "")

	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")

	v1 := filepath.Join(home, ".claude", "skills", "skill-x")
	mustMkdirAll(t, v1)
	if err := os.WriteFile(filepath.Join(v1, "SKILL.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	v2 := filepath.Join(home, ".cursor", "skills", "skill-x")
	mustMkdirAll(t, v2)
	if err := os.WriteFile(filepath.Join(v2, "SKILL.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"name":        "skill-x",
		"target":      "codex",
		"auto_import": true,
	})
	res, err := http.Post(srv.URL+"/api/skills/link", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post link: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d, want 400, body=%s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	b, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(b), "MULTI_VARIANTS|") {
		t.Fatalf("body=%q, want MULTI_VARIANTS|", strings.TrimSpace(string(b)))
	}
}

func TestAPI_Skills_ListRepoFilterAndFacet(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "alpha-one"))
	mustMkdirAll(t, filepath.Join(sourceRoot, "alpha-two"))
	mustMkdirAll(t, filepath.Join(sourceRoot, "beta"))
	mustMkdirAll(t, filepath.Join(sourceRoot, "local"))

	mustWriteManagedManifestJSON(t, filepath.Join(sourceRoot, "alpha-one"), map[string]any{
		"schema_version": 1,
		"name":           "alpha-one",
		"source_type":    "git",
		"source_ref":     "https://github.com/acme/repo-a.git",
	})
	mustWriteManagedManifestJSON(t, filepath.Join(sourceRoot, "alpha-two"), map[string]any{
		"schema_version": 1,
		"name":           "alpha-two",
		"source_type":    "git",
		"source_ref":     "acme/repo-a",
	})
	mustWriteManagedManifestJSON(t, filepath.Join(sourceRoot, "beta"), map[string]any{
		"schema_version": 1,
		"name":           "beta",
		"source_type":    "git",
		"source_ref":     "https://github.com/acme/repo-b",
	})
	mustWriteManagedManifestJSON(t, filepath.Join(sourceRoot, "local"), map[string]any{
		"schema_version": 1,
		"name":           "local",
		"source_type":    "local",
		"source_ref":     filepath.Join(home, "src-local"),
	})

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	type page struct {
		Skills []skills.Skill     `json:"skills"`
		Repos  []skills.RepoFacet `json:"repos"`
		Total  int                `json:"total"`
		Offset int                `json:"offset"`
		Limit  int                `json:"limit"`
	}

	all := page{}
	{
		res, err := http.Get(srv.URL + "/api/skills")
		if err != nil {
			t.Fatalf("get all: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(&all); err != nil {
			t.Fatalf("decode all: %v", err)
		}
	}
	if all.Total != 4 {
		t.Fatalf("total=%d, want 4", all.Total)
	}
	if len(all.Repos) != 2 {
		t.Fatalf("repos=%d, want 2 (%+v)", len(all.Repos), all.Repos)
	}

	repoA := ""
	for _, r := range all.Repos {
		if r.Count == 2 {
			repoA = r.Key
		}
	}
	if repoA == "" {
		t.Fatalf("repoA key not found in facets: %+v", all.Repos)
	}

	{
		res, err := http.Get(srv.URL + "/api/skills?repo=" + repoA)
		if err != nil {
			t.Fatalf("get repo: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var got page
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatalf("decode repo: %v", err)
		}
		if got.Total != 2 || len(got.Skills) != 2 {
			t.Fatalf("repo filter total=%d skills=%d, want 2/2", got.Total, len(got.Skills))
		}
		for _, s := range got.Skills {
			if s.RepoKey != repoA {
				t.Fatalf("skill %s repo_key=%q, want %q", s.Name, s.RepoKey, repoA)
			}
		}
		if len(got.Repos) != 2 {
			t.Fatalf("facet should keep all repos, got=%d", len(got.Repos))
		}
	}

	{
		res, err := http.Get(srv.URL + "/api/skills?repo=" + repoA + "&q=two")
		if err != nil {
			t.Fatalf("get repo+q: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var got page
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatalf("decode repo+q: %v", err)
		}
		if got.Total != 1 || len(got.Skills) != 1 {
			t.Fatalf("repo+q total=%d skills=%d, want 1/1", got.Total, len(got.Skills))
		}
		if got.Skills[0].Name != "alpha-two" {
			t.Fatalf("skill=%q, want alpha-two", got.Skills[0].Name)
		}
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func mustWriteManagedManifestJSON(t *testing.T, skillDir string, payload map[string]any) {
	t.Helper()
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	b = append(b, '\n')
	path := filepath.Join(skillDir, ".controlccx_skill.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write manifest %s: %v", path, err)
	}
}
