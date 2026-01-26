package worker

import (
	"runtime"
	"strings"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestBuildClaude_DoesNotDisableSettingSources(t *testing.T) {
	cfg := config.Default()
	task := tasks.Task{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		WorkDir:    ".",
		Prompt:     "hi",
	}

	tool, err := buildClaude(cfg, task)
	if err != nil {
		t.Fatalf("buildClaude: %v", err)
	}

	if runtime.GOOS == "windows" {
		if len(tool.Args) < 2 || tool.Args[0] != "-lc" {
			t.Fatalf("windows claude wrapper args=%v, want [-lc <cmd>]", tool.Args)
		}
		if strings.Contains(tool.Args[1], "--setting-sources") {
			t.Fatalf("windows wrapped command contains --setting-sources: %q", tool.Args[1])
		}
		return
	}

	for _, arg := range tool.Args {
		if arg == "--setting-sources" {
			t.Fatalf("unexpected --setting-sources in args: %v", tool.Args)
		}
	}
}
