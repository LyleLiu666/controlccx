package worker

import (
	"path/filepath"
	"testing"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

func TestManager_buildToolCommand_CodexDefaults(t *testing.T) {
	m := &Manager{cfg: config.Default()}
	task := tasks.Task{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		WorkDir:    ".",
		Prompt:     "hi",
	}

	tool, _, err := m.buildToolCommand(task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantModel := "gpt-5.2"
	wantCfg := `model_reasoning_effort="xhigh"`

	execIdx := indexOfAny(tool.Args, "e", "exec")
	if execIdx < 0 {
		t.Fatalf("args=%v, expected exec subcommand", tool.Args)
	}
	if len(tool.Args) < execIdx+5 {
		t.Fatalf("args=%v, want >= %d", tool.Args, execIdx+5)
	}
	if tool.Args[execIdx+1] != "-m" || tool.Args[execIdx+2] != wantModel || tool.Args[execIdx+3] != "-c" || tool.Args[execIdx+4] != wantCfg {
		t.Fatalf("args near exec=%v, want [exec -m %s -c %s]", tool.Args[execIdx:min(execIdx+5, len(tool.Args))], wantModel, wantCfg)
	}
}

func TestManager_buildToolCommand_CodexUsesStoredModelAndEffort(t *testing.T) {
	store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	model := "o3"
	effort := "high"
	if _, err := store.ApplyPatch(auth.Patch{CodexModel: &model, CodexReasoningEffort: &effort}); err != nil {
		t.Fatalf("store.ApplyPatch: %v", err)
	}

	m := &Manager{cfg: config.Default(), auth: store}
	task := tasks.Task{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		WorkDir:    ".",
		Prompt:     "hi",
	}

	tool, _, err := m.buildToolCommand(task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantCfg := `model_reasoning_effort="high"`

	execIdx := indexOfAny(tool.Args, "e", "exec")
	if execIdx < 0 {
		t.Fatalf("args=%v, expected exec subcommand", tool.Args)
	}
	if len(tool.Args) < execIdx+5 {
		t.Fatalf("args=%v, want >= %d", tool.Args, execIdx+5)
	}
	if tool.Args[execIdx+1] != "-m" || tool.Args[execIdx+2] != "o3" || tool.Args[execIdx+3] != "-c" || tool.Args[execIdx+4] != wantCfg {
		t.Fatalf("args near exec=%v, want [exec -m o3 -c %s]", tool.Args[execIdx:min(execIdx+5, len(tool.Args))], wantCfg)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func indexOfAny(items []string, values ...string) int {
	for i, it := range items {
		for _, v := range values {
			if it == v {
				return i
			}
		}
	}
	return -1
}
