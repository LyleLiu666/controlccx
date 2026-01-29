package worker

import (
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestBuildToolCommand_Default_DoesNotUseDangerousFlags(t *testing.T) {
	cfg := config.Default()

	claudeTask := tasks.Task{WorkerType: tasks.WorkerClaudeCode, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: "."}
	claudeTool, err := BuildToolCommand(cfg, claudeTask)
	if err != nil {
		t.Fatalf("BuildToolCommand claude: %v", err)
	}
	if hasArg(claudeTool.Args, "--dangerously-skip-permissions") {
		t.Fatalf("unexpected claude dangerous flag in args=%v", claudeTool.Args)
	}

	codexTask := tasks.Task{WorkerType: tasks.WorkerCodex, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: "."}
	codexTool, err := BuildToolCommand(cfg, codexTask)
	if err != nil {
		t.Fatalf("BuildToolCommand codex: %v", err)
	}
	if hasArg(codexTool.Args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("unexpected codex dangerous flag in args=%v", codexTool.Args)
	}
	if !hasArg(codexTool.Args, "--sandbox") || !hasArg(codexTool.Args, "workspace-write") {
		t.Fatalf("expected codex sandbox workspace-write in args=%v", codexTool.Args)
	}
}

func TestBuildToolCommand_UnsafeAutomation_UsesDangerousFlags(t *testing.T) {
	cfg := config.Default()
	cfg.Workers.UnsafeAutomation = true

	claudeTask := tasks.Task{WorkerType: tasks.WorkerClaudeCode, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: "."}
	claudeTool, err := BuildToolCommand(cfg, claudeTask)
	if err != nil {
		t.Fatalf("BuildToolCommand claude: %v", err)
	}
	if !hasArg(claudeTool.Args, "--dangerously-skip-permissions") {
		t.Fatalf("expected claude dangerous flag in args=%v", claudeTool.Args)
	}

	codexTask := tasks.Task{WorkerType: tasks.WorkerCodex, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: "."}
	codexTool, err := BuildToolCommand(cfg, codexTask)
	if err != nil {
		t.Fatalf("BuildToolCommand codex: %v", err)
	}
	if !hasArg(codexTool.Args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected codex dangerous flag in args=%v", codexTool.Args)
	}
	if hasArg(codexTool.Args, "--sandbox") {
		t.Fatalf("unexpected codex sandbox flag in args=%v", codexTool.Args)
	}
}

func TestBuildToolCommand_PerRunUnsafeAutomation_UsesDangerousFlags(t *testing.T) {
	cfg := config.Default()

	claudeTask := tasks.Task{WorkerType: tasks.WorkerClaudeCode, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: ".", UnsafeAutomation: true}
	claudeTool, err := BuildToolCommand(cfg, claudeTask)
	if err != nil {
		t.Fatalf("BuildToolCommand claude: %v", err)
	}
	if !hasArg(claudeTool.Args, "--dangerously-skip-permissions") {
		t.Fatalf("expected claude dangerous flag in args=%v", claudeTool.Args)
	}

	codexTask := tasks.Task{WorkerType: tasks.WorkerCodex, Mode: tasks.ModeNew, Prompt: "hi", WorkDir: ".", UnsafeAutomation: true}
	codexTool, err := BuildToolCommand(cfg, codexTask)
	if err != nil {
		t.Fatalf("BuildToolCommand codex: %v", err)
	}
	if !hasArg(codexTool.Args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected codex dangerous flag in args=%v", codexTool.Args)
	}
	if hasArg(codexTool.Args, "--sandbox") {
		t.Fatalf("unexpected codex sandbox flag in args=%v", codexTool.Args)
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
