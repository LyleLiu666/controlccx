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

	tool, err := m.buildToolCommand(task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantModel := "gpt-5.2"
	wantCfg := `model_reasoning_effort="xhigh"`

	if len(tool.Args) < 5 {
		t.Fatalf("args=%v, want >= 5", tool.Args)
	}
	if tool.Args[0] != "e" || tool.Args[1] != "-m" || tool.Args[2] != wantModel || tool.Args[3] != "-c" || tool.Args[4] != wantCfg {
		t.Fatalf("args prefix=%v, want [e -m %s -c %s]", tool.Args[:min(5, len(tool.Args))], wantModel, wantCfg)
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

	tool, err := m.buildToolCommand(task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantCfg := `model_reasoning_effort="high"`

	if len(tool.Args) < 5 {
		t.Fatalf("args=%v, want >= 5", tool.Args)
	}
	if tool.Args[0] != "e" || tool.Args[1] != "-m" || tool.Args[2] != "o3" || tool.Args[3] != "-c" || tool.Args[4] != wantCfg {
		t.Fatalf("args prefix=%v, want [e -m o3 -c %s]", tool.Args[:min(5, len(tool.Args))], wantCfg)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

