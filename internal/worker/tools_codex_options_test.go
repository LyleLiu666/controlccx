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

	if !hasArg(tool.Args, "app-server") {
		t.Fatalf("args=%v, expected app-server subcommand", tool.Args)
	}
	// Approval policy is configured via JSON-RPC params (thread/start, thread/resume), not CLI args.
	if hasArg(tool.Args, "--ask-for-approval") {
		t.Fatalf("args=%v, unexpected --ask-for-approval", tool.Args)
	}
}

func TestBuildToolCommand_CodexSearch_EnablesFeatureBeforeAppServer(t *testing.T) {
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

	appIdx := indexOf(tool.Args, "app-server")
	if appIdx < 0 {
		t.Fatalf("args=%v, expected app-server subcommand", tool.Args)
	}
	enableIdx := indexOf(tool.Args, "--enable")
	if enableIdx < 0 {
		t.Fatalf("args=%v, expected --enable web_search_request", tool.Args)
	}
	if enableIdx > appIdx {
		t.Fatalf("args=%v, expected --enable before app-server", tool.Args)
	}
	if enableIdx+1 >= len(tool.Args) || tool.Args[enableIdx+1] != "web_search_request" {
		t.Fatalf("args=%v, expected --enable web_search_request", tool.Args)
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
