package worker

import (
	"context"
	"runtime"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
)

func TestManager_buildToolCommand_Exec_WithToolingConfigured(t *testing.T) {
	toolsSvc, err := tooling.NewService(tooling.Options{
		DataDir: t.TempDir(),
		Defaults: []tooling.Tool{
			{ID: "claude-code", Driver: tooling.DriverClaudeCode, Command: "claude"},
			{ID: "codex", Driver: tooling.DriverCodex, Command: "codex"},
		},
	})
	if err != nil {
		t.Fatalf("new tools service: %v", err)
	}

	m := &Manager{
		cfg:   config.Default(),
		tools: toolsSvc,
	}

	tool, driver, err := m.buildToolCommand(context.Background(), tasks.Task{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo hello",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("build tool command: %v", err)
	}
	if driver != tasks.WorkerExec {
		t.Fatalf("driver=%q want %q", driver, tasks.WorkerExec)
	}
	wantCmd := "sh"
	if runtime.GOOS == "windows" {
		wantCmd = "cmd.exe"
	}
	if tool.Command != wantCmd {
		t.Fatalf("cmd=%q want %q", tool.Command, wantCmd)
	}
}
