package worker

import (
	"testing"

	"controlccx/internal/tasks"
)

func TestNormalizeSkillTokensForExecution_Codex_RewritesSlashTokens(t *testing.T) {
	known := map[string]bool{"code-review-excellence": true}
	in := "/code-review-excellence\nDo the thing\n"
	out, changes := normalizeSkillTokensForExecution(tasks.WorkerCodex, in, known)
	if changes != 1 {
		t.Fatalf("changes=%d, want 1", changes)
	}
	if out != "$code-review-excellence\nDo the thing\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNormalizeSkillTokensForExecution_Codex_DoesNotRewritePaths(t *testing.T) {
	known := map[string]bool{"Users": true}
	in := "/Users/alice/project\n"
	out, changes := normalizeSkillTokensForExecution(tasks.WorkerCodex, in, known)
	if changes != 0 {
		t.Fatalf("changes=%d, want 0", changes)
	}
	if out != in {
		t.Fatalf("out=%q, want %q", out, in)
	}
}

func TestNormalizeSkillTokensForExecution_Claude_RewritesDollarTokens(t *testing.T) {
	known := map[string]bool{"code-review-excellence": true}
	in := "Use $code-review-excellence now\n"
	out, changes := normalizeSkillTokensForExecution(tasks.WorkerClaudeCode, in, known)
	if changes != 1 {
		t.Fatalf("changes=%d, want 1", changes)
	}
	if out != "Use /code-review-excellence now\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNormalizeSkillTokensForExecution_Claude_DoesNotRewriteShellVars(t *testing.T) {
	known := map[string]bool{"code-review-excellence": true}
	in := "$HOME\n"
	out, changes := normalizeSkillTokensForExecution(tasks.WorkerClaudeCode, in, known)
	if changes != 0 {
		t.Fatalf("changes=%d, want 0", changes)
	}
	if out != in {
		t.Fatalf("out=%q, want %q", out, in)
	}
}

func TestNormalizeSkillTokensForExecution_RequiresTokenBoundaries(t *testing.T) {
	known := map[string]bool{"code-review-excellence": true}

	in := "abc/code-review-excellence \n"
	out, changes := normalizeSkillTokensForExecution(tasks.WorkerCodex, in, known)
	if changes != 0 {
		t.Fatalf("changes=%d, want 0", changes)
	}
	if out != in {
		t.Fatalf("out=%q, want %q", out, in)
	}

	in = "/code-review-excellence/extra\n"
	out, changes = normalizeSkillTokensForExecution(tasks.WorkerCodex, in, known)
	if changes != 0 {
		t.Fatalf("changes=%d, want 0", changes)
	}
	if out != in {
		t.Fatalf("out=%q, want %q", out, in)
	}
}

