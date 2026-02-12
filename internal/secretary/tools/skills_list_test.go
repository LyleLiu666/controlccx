package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/skills"
)

func TestTools_SkillsList_FiltersEnabledByTarget(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "bravo"), 0o755); err != nil {
		t.Fatalf("mkdir bravo: %v", err)
	}

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
		CodexHome:   filepath.Join(home, ".codex2"),
	})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	if err := svc.Link(ctx, "alpha", skills.TargetCodex); err != nil {
		t.Fatalf("link alpha: %v", err)
	}

	reg := NewRegistry(Deps{Skills: svc})
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "skills_list",
		Fields: map[string]string{
			"target":       "codex",
			"only_enabled": "1",
		},
	})
	if err != nil {
		t.Fatalf("execute skills_list: %v", err)
	}

	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type %T", outAny)
	}
	raw, err := json.Marshal(out["skills"])
	if err != nil {
		t.Fatalf("marshal skills payload: %v", err)
	}
	var got []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal skills payload: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("unexpected skills=%s", string(raw))
	}
}

func TestTools_SkillsList_RejectsUnknownTarget(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}

	svc, err := skills.NewService(skills.Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}

	reg := NewRegistry(Deps{Skills: svc})
	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "skills_list",
		Fields: map[string]string{"target": "not-a-target"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTools_SkillsList_QFilterAndPagination(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if err := os.MkdirAll(filepath.Join(sourceRoot, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	svc, err := skills.NewService(skills.Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	reg := NewRegistry(Deps{Skills: svc})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "skills_list",
		Fields: map[string]string{
			"q": "br",
		},
	})
	if err != nil {
		t.Fatalf("execute skills_list q: %v", err)
	}
	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type %T", outAny)
	}
	raw, err := json.Marshal(out["skills"])
	if err != nil {
		t.Fatalf("marshal skills payload: %v", err)
	}
	var got []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal skills payload: %v", err)
	}
	if len(got) != 1 || got[0].Name != "bravo" {
		t.Fatalf("unexpected skills=%s", string(raw))
	}

	outAny, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "skills_list",
		Fields: map[string]string{
			"limit":  "1",
			"offset": "1",
		},
	})
	if err != nil {
		t.Fatalf("execute skills_list paginate: %v", err)
	}
	out, ok = outAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type %T", outAny)
	}
	raw, err = json.Marshal(out["skills"])
	if err != nil {
		t.Fatalf("marshal skills payload: %v", err)
	}
	got = nil
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal skills payload: %v", err)
	}
	if len(got) != 1 || got[0].Name != "bravo" {
		t.Fatalf("unexpected skills=%s", string(raw))
	}
}

func TestTools_SkillsList_IncludePathsControlsOutputSize(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".agent", "skills")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}

	svc, err := skills.NewService(skills.Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
		CodexHome:   filepath.Join(home, ".codex2"),
	})
	if err != nil {
		t.Fatalf("new skills service: %v", err)
	}
	if err := svc.Link(ctx, "alpha", skills.TargetCodex); err != nil {
		t.Fatalf("link alpha: %v", err)
	}

	reg := NewRegistry(Deps{Skills: svc})
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "skills_list",
		Fields: map[string]string{
			"target":       "codex",
			"only_enabled": "1",
		},
	})
	if err != nil {
		t.Fatalf("execute skills_list: %v", err)
	}
	raw, _ := json.Marshal(outAny)
	if strings.Contains(string(raw), home) {
		t.Fatalf("expected paths excluded by default, got=%s", string(raw))
	}

	outAny, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "skills_list",
		Fields: map[string]string{
			"target":        "codex",
			"only_enabled":  "1",
			"include_paths": "1",
		},
	})
	if err != nil {
		t.Fatalf("execute skills_list include_paths: %v", err)
	}
	raw, _ = json.Marshal(outAny)
	if !strings.Contains(string(raw), home) {
		t.Fatalf("expected paths included when include_paths=1, got=%s", string(raw))
	}
}
