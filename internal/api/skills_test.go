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

	"controlccx/internal/skills"
)

func TestAPI_Skills_ListLinkUnlink(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agents", "skills")
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
	sourceRoot := filepath.Join(home, ".agents", "skills")
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

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}
