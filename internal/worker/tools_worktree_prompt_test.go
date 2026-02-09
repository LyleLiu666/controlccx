package worker

import (
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestBuildToolCommand_WorktreeStrategy_PrependsScopeHeader_Codex(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:      tasks.WorkerCodex,
		Mode:            tasks.ModeNew,
		Prompt:          "hi",
		WorkDir:         "/tmp/repo/.ccx/worktrees/cid/abc",
		WorkDirStrategy: "worktree",
		BaseWorkDir:     "/tmp/repo",
		WorktreeDir:     "/tmp/repo/.ccx/worktrees/cid/abc",
		WorktreeBranch:  "ccx/cid/abc",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}
	if !strings.Contains(tool.Stdin, worktreePromptSentinel) {
		t.Fatalf("stdin=%q, expected sentinel %q", tool.Stdin, worktreePromptSentinel)
	}
	if !strings.Contains(tool.Stdin, task.BaseWorkDir) {
		t.Fatalf("stdin=%q, expected base_workdir %q", tool.Stdin, task.BaseWorkDir)
	}
	if !strings.Contains(tool.Stdin, task.WorktreeBranch) {
		t.Fatalf("stdin=%q, expected worktree_branch %q", tool.Stdin, task.WorktreeBranch)
	}
	if !strings.Contains(tool.Stdin, "\n\n---\n\n"+task.Prompt) {
		t.Fatalf("stdin=%q, expected original prompt preserved", tool.Stdin)
	}
}

func TestBuildToolCommand_WorktreeStrategy_DoesNotDuplicateHeader(t *testing.T) {
	cfg := config.Default()
	prompt := "[" + worktreePromptSentinel + "]\n\nhi"
	task := tasks.Task{
		WorkerType:      tasks.WorkerCodex,
		Mode:            tasks.ModeNew,
		Prompt:          prompt,
		WorkDir:         "/tmp/repo/.ccx/worktrees/cid/abc",
		WorkDirStrategy: "worktree",
		BaseWorkDir:     "/tmp/repo",
		WorktreeDir:     "/tmp/repo/.ccx/worktrees/cid/abc",
		WorktreeBranch:  "ccx/cid/abc",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}
	if tool.Stdin != prompt {
		t.Fatalf("stdin=%q, want prompt unchanged=%q", tool.Stdin, prompt)
	}
}

func TestBuildToolCommand_WorkspaceStrategy_PrependsScopeHeader_Claude(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:             tasks.WorkerClaudeCode,
		Mode:                   tasks.ModeNew,
		Prompt:                 "hi",
		WorkDir:                "/tmp/repo/.ccx/workspaces/cid/copy",
		WorkDirStrategy:        "workspace",
		BaseWorkDir:            "/tmp/repo",
		WorktreeDir:            "/tmp/repo/.ccx/workspaces/cid/copy",
		RunWorkspaceKind:       "copy",
		RunWorkspaceBaseBranch: "main",
		RunWorkspaceWorkBranch: "ccx/cid/ws",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}
	if !strings.Contains(tool.Stdin, workspacePromptSentinel) {
		t.Fatalf("stdin=%q, expected sentinel %q", tool.Stdin, workspacePromptSentinel)
	}
	if !strings.Contains(tool.Stdin, task.BaseWorkDir) {
		t.Fatalf("stdin=%q, expected base_workdir %q", tool.Stdin, task.BaseWorkDir)
	}
	if !strings.Contains(tool.Stdin, task.WorktreeDir) {
		t.Fatalf("stdin=%q, expected run_workdir %q", tool.Stdin, task.WorktreeDir)
	}
	if !strings.Contains(tool.Stdin, task.RunWorkspaceBaseBranch) {
		t.Fatalf("stdin=%q, expected base_branch %q", tool.Stdin, task.RunWorkspaceBaseBranch)
	}
	if !strings.Contains(tool.Stdin, task.RunWorkspaceWorkBranch) {
		t.Fatalf("stdin=%q, expected work_branch %q", tool.Stdin, task.RunWorkspaceWorkBranch)
	}
	if !strings.Contains(tool.Stdin, "\n\n---\n\n"+task.Prompt) {
		t.Fatalf("stdin=%q, expected original prompt preserved", tool.Stdin)
	}
}

func TestBuildToolCommand_WorkspaceStrategy_DoesNotDuplicateHeader(t *testing.T) {
	cfg := config.Default()
	prompt := "[" + workspacePromptSentinel + "]\n\nhi"
	task := tasks.Task{
		WorkerType:             tasks.WorkerClaudeCode,
		Mode:                   tasks.ModeNew,
		Prompt:                 prompt,
		WorkDir:                "/tmp/repo/.ccx/workspaces/cid/copy",
		WorkDirStrategy:        "workspace",
		BaseWorkDir:            "/tmp/repo",
		WorktreeDir:            "/tmp/repo/.ccx/workspaces/cid/copy",
		RunWorkspaceKind:       "copy",
		RunWorkspaceBaseBranch: "main",
		RunWorkspaceWorkBranch: "ccx/cid/ws",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}
	if tool.Stdin != prompt {
		t.Fatalf("stdin=%q, want prompt unchanged=%q", tool.Stdin, prompt)
	}
}
