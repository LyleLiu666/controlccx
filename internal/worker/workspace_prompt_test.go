package worker

import (
	"strings"
	"testing"

	"controlccx/internal/tasks"
)

func TestInjectWorkspaceContextPrompt_PrefixesBaseAndRun(t *testing.T) {
	ws := tasks.SessionWorkspace{
		Kind:        tasks.WorkspaceKindCopy,
		BaseWorkDir: "/base",
		RunWorkDir:  "/run/.ccx/workspaces/abc/copy",
	}
	prompt := "do the thing"

	out := injectWorkspaceContextPrompt(prompt, ws)
	if !strings.HasSuffix(out, prompt) {
		t.Fatalf("expected output to end with original prompt")
	}
	if !strings.Contains(out, "[controlccx workspace]") {
		t.Fatalf("missing workspace marker")
	}
	if !strings.Contains(out, "isolated workspace (copy)") {
		t.Fatalf("missing kind")
	}
	if !strings.Contains(out, "base_workdir: /base") {
		t.Fatalf("missing base_workdir")
	}
	if !strings.Contains(out, "run_workdir: /run/.ccx/workspaces/abc/copy") {
		t.Fatalf("missing run_workdir")
	}
}

func TestInjectWorkspaceContextPrompt_SkipsIfAlreadyPresent(t *testing.T) {
	ws := tasks.SessionWorkspace{
		Kind:        tasks.WorkspaceKindCopy,
		BaseWorkDir: "/base",
		RunWorkDir:  "/run",
	}
	prompt := "[controlccx workspace]\nhello"
	out := injectWorkspaceContextPrompt(prompt, ws)
	if out != prompt {
		t.Fatalf("expected prompt to remain unchanged when marker present")
	}
}

func TestInjectWorkspaceContextPrompt_IgnoresIncompleteWorkspace(t *testing.T) {
	ws := tasks.SessionWorkspace{
		Kind:        tasks.WorkspaceKindCopy,
		BaseWorkDir: "",
		RunWorkDir:  "/run",
	}
	prompt := "x"
	out := injectWorkspaceContextPrompt(prompt, ws)
	if out != prompt {
		t.Fatalf("expected prompt to remain unchanged for incomplete workspace info")
	}
}
