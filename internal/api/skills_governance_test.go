package api

import (
	"bytes"
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

func TestAPI_SkillsGovernance_ToolsOnboardingInstallSyncUpdate(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdirAll(t, filepath.Join(sourceRoot, "demo"))
	mustWriteFile(t, filepath.Join(sourceRoot, "demo", "SKILL.md"), "v1\n")

	// Mark tools as installed.
	mustMkdirAll(t, filepath.Join(home, ".cursor"))
	mustMkdirAll(t, filepath.Join(home, ".claude"))
	mustMkdirAll(t, filepath.Join(home, ".codex"))

	// Seed onboarding sources (unmanaged).
	mustMkdirAll(t, filepath.Join(home, ".cursor", "skills", "x"))
	mustWriteFile(t, filepath.Join(home, ".cursor", "skills", "x", "a.txt"), "cursor\n")
	mustMkdirAll(t, filepath.Join(home, ".codex", "skills", "x"))
	mustWriteFile(t, filepath.Join(home, ".codex", "skills", "x", "a.txt"), "codex\n")

	svc, err := skills.NewService(skills.Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills: %v", err)
	}

	apiSvc := &API{Skills: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// Tools
	{
		res, err := http.Get(srv.URL + "/api/skills/tools")
		if err != nil {
			t.Fatalf("get tools: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("tools status=%d", res.StatusCode)
		}
		var body struct {
			Tools []skills.ToolInfo `json:"tools"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode tools: %v", err)
		}
		if len(body.Tools) != 5 {
			t.Fatalf("tools=%v", body.Tools)
		}
		installed := map[string]bool{}
		for _, tool := range body.Tools {
			installed[tool.Key] = tool.Installed
		}
		if !installed["cursor"] || !installed["claude_code"] || !installed["codex"] {
			t.Fatalf("expected cursor/claude_code/codex installed, got=%v", installed)
		}
		if installed["antigravity"] || installed["opencode"] {
			t.Fatalf("expected antigravity/opencode not installed, got=%v", installed)
		}
	}

	// Onboarding
	{
		res, err := http.Get(srv.URL + "/api/skills/onboarding")
		if err != nil {
			t.Fatalf("get onboarding: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("onboarding status=%d", res.StatusCode)
		}
		var plan skills.OnboardingPlan
		if err := json.NewDecoder(res.Body).Decode(&plan); err != nil {
			t.Fatalf("decode onboarding: %v", err)
		}
		if plan.TotalToolsScanned != 3 {
			t.Fatalf("TotalToolsScanned=%d", plan.TotalToolsScanned)
		}
		if plan.TotalSkillsFound != 2 {
			t.Fatalf("TotalSkillsFound=%d", plan.TotalSkillsFound)
		}
		if len(plan.Groups) != 1 || plan.Groups[0].Name != "x" {
			t.Fatalf("groups=%v", plan.Groups)
		}
		if !plan.Groups[0].HasConflict {
			t.Fatalf("expected conflict for x")
		}
	}

	// Install local via API
	localSrc := filepath.Join(home, "local-src")
	mustMkdirAll(t, localSrc)
	mustWriteFile(t, filepath.Join(localSrc, "SKILL.md"), "local\n")

	{
		buf, _ := json.Marshal(map[string]any{
			"source_path": localSrc,
			"name":        "local-skill",
		})
		res, err := http.Post(srv.URL+"/api/skills/install/local", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("install local: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("install local status=%d body=%s", res.StatusCode, string(b))
		}
		if _, err := os.Stat(filepath.Join(sourceRoot, "local-skill", ".controlccx_skill.json")); err != nil {
			t.Fatalf("expected manifest: %v", err)
		}
	}

	{
		// Installing again should be non-destructive.
		buf, _ := json.Marshal(map[string]any{"source_path": localSrc, "name": "local-skill"})
		res, err := http.Post(srv.URL+"/api/skills/install/local", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("install local again: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got=%d", res.StatusCode)
		}
		b, _ := io.ReadAll(res.Body)
		if !strings.HasPrefix(string(b), "TARGET_EXISTS|") {
			t.Fatalf("expected TARGET_EXISTS, got=%q", string(b))
		}
	}

	// Sync overwrite (cursor forces copy)
	{
		mustMkdirAll(t, filepath.Join(home, ".cursor", "skills", "demo"))
		mustWriteFile(t, filepath.Join(home, ".cursor", "skills", "demo", "unmanaged.txt"), "x\n")

		buf, _ := json.Marshal(map[string]any{"name": "demo", "target": "cursor"})
		res, err := http.Post(srv.URL+"/api/skills/sync", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("sync: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got=%d", res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if !strings.HasPrefix(string(body), "TARGET_EXISTS|") {
			t.Fatalf("expected TARGET_EXISTS, got=%q", string(body))
		}

		buf, _ = json.Marshal(map[string]any{"name": "demo", "target": "cursor", "overwrite": true})
		res2, err := http.Post(srv.URL+"/api/skills/sync", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("sync overwrite: %v", err)
		}
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(res2.Body)
			t.Fatalf("sync overwrite status=%d body=%s", res2.StatusCode, string(b))
		}
		if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "demo", ".controlccx_skill_source")); err != nil {
			t.Fatalf("expected managed marker: %v", err)
		}
	}

	// Update managed skill
	{
		mustWriteFile(t, filepath.Join(localSrc, "SKILL.md"), "local-v2\n")
		buf, _ := json.Marshal(map[string]any{"name": "local-skill"})
		res, err := http.Post(srv.URL+"/api/skills/update", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("update status=%d body=%s", res.StatusCode, string(b))
		}
		b, err := os.ReadFile(filepath.Join(sourceRoot, "local-skill", "SKILL.md"))
		if err != nil {
			t.Fatalf("read updated: %v", err)
		}
		if strings.TrimSpace(string(b)) != "local-v2" {
			t.Fatalf("expected updated content, got=%q", string(b))
		}
	}
}
