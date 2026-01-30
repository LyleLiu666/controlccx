package worker

import (
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestBuildToolCommand_CodexApprovalPolicy_IsGlobalBeforeExec(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:          tasks.WorkerCodex,
		Mode:                tasks.ModeNew,
		Prompt:              "hi",
		WorkDir:             ".",
		CodexApprovalPolicy: "untrusted",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	execIdx := indexOfAny(tool.Args, "e", "exec")
	if execIdx < 0 {
		t.Fatalf("args=%v, expected exec subcommand", tool.Args)
	}

	apprIdx := indexOf(tool.Args, "--ask-for-approval")
	if apprIdx < 0 {
		t.Fatalf("args=%v, expected --ask-for-approval", tool.Args)
	}
	if apprIdx > execIdx {
		t.Fatalf("args=%v, expected --ask-for-approval before exec", tool.Args)
	}
	if apprIdx+1 >= len(tool.Args) || tool.Args[apprIdx+1] != "untrusted" {
		t.Fatalf("args=%v, expected --ask-for-approval untrusted", tool.Args)
	}
}

func TestBuildToolCommand_CodexSearch_IsGlobalBeforeExec(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType:   tasks.WorkerCodex,
		Mode:         tasks.ModeNew,
		Prompt:       "hi",
		WorkDir:      ".",
		CodexSearch:  true,
		CodexSandbox: "workspace-write",
	}

	tool, err := BuildToolCommand(cfg, task)
	if err != nil {
		t.Fatalf("BuildToolCommand: %v", err)
	}

	execIdx := indexOfAny(tool.Args, "e", "exec")
	if execIdx < 0 {
		t.Fatalf("args=%v, expected exec subcommand", tool.Args)
	}

	searchIdx := indexOf(tool.Args, "--search")
	if searchIdx < 0 {
		t.Fatalf("args=%v, expected --search", tool.Args)
	}
	if searchIdx > execIdx {
		t.Fatalf("args=%v, expected --search before exec", tool.Args)
	}
}

func indexOf(items []string, value string) int {
	for i, it := range items {
		if it == value {
			return i
		}
	}
	return -1
}

