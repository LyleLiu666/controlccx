package skills

import (
	"context"
	"path/filepath"
	"testing"
)

func TestService_OnboardingPlan_GroupsAndDetectsConflicts(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	// Canonical store (not required for scanning, but used as source root).
	sourceRoot := filepath.Join(home, ".agent", "skills")
	mustMkdir(t, sourceRoot)

	// Cursor
	mustMkdir(t, filepath.Join(home, ".cursor"))
	cursorSkills := filepath.Join(home, ".cursor", "skills")
	mustMkdir(t, filepath.Join(cursorSkills, "x"))
	mustWrite(t, filepath.Join(cursorSkills, "x", "a.txt"), "cursor\n")

	// Claude Code
	mustMkdir(t, filepath.Join(home, ".claude"))
	claudeSkills := filepath.Join(home, ".claude", "skills")
	mustMkdir(t, filepath.Join(claudeSkills, "y"))
	mustWrite(t, filepath.Join(claudeSkills, "y", "a.txt"), "claude\n")

	// Codex
	mustMkdir(t, filepath.Join(home, ".codex"))
	codexSkills := filepath.Join(home, ".codex", "skills")
	mustMkdir(t, filepath.Join(codexSkills, "x"))
	mustWrite(t, filepath.Join(codexSkills, "x", "a.txt"), "codex\n")
	mustMkdir(t, filepath.Join(codexSkills, ".system"))

	svc, err := NewService(Options{
		HomeDir:     home,
		SourceRoots: []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	plan, err := svc.OnboardingPlan(ctx)
	if err != nil {
		t.Fatalf("onboarding plan: %v", err)
	}
	if plan.TotalToolsScanned != 3 {
		t.Fatalf("TotalToolsScanned=%d", plan.TotalToolsScanned)
	}
	if plan.TotalSkillsFound != 3 {
		t.Fatalf("TotalSkillsFound=%d", plan.TotalSkillsFound)
	}
	if len(plan.Groups) != 2 {
		t.Fatalf("groups=%v", plan.Groups)
	}

	var groupX, groupY *OnboardingGroup
	for i := range plan.Groups {
		g := &plan.Groups[i]
		switch g.Name {
		case "x":
			groupX = g
		case "y":
			groupY = g
		}
	}
	if groupX == nil || groupY == nil {
		t.Fatalf("expected groups x and y, got=%v", plan.Groups)
	}
	if !groupX.HasConflict {
		t.Fatalf("expected conflict for x")
	}
	if groupY.HasConflict {
		t.Fatalf("expected no conflict for y")
	}
	if len(groupX.Variants) != 2 {
		t.Fatalf("x variants=%v", groupX.Variants)
	}
	for _, v := range groupX.Variants {
		if v.Fingerprint == "" {
			t.Fatalf("expected fingerprint for %v", v)
		}
	}
}
