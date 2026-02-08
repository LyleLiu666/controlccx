package worker

import (
	"context"
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

	tool, _, err := m.buildToolCommand(context.Background(), task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantModel := "gpt-5.2"
	wantCfg := `model_reasoning_effort="xhigh"`

	appIdx := indexOfAny(tool.Args, "app-server")
	if appIdx < 0 {
		t.Fatalf("args=%v, expected app-server subcommand", tool.Args)
	}
	if appIdx < 4 {
		t.Fatalf("args=%v, want defaults before app-server", tool.Args)
	}
	if tool.Args[appIdx-4] != "-m" || tool.Args[appIdx-3] != wantModel || tool.Args[appIdx-2] != "-c" || tool.Args[appIdx-1] != wantCfg {
		t.Fatalf("args=%v, want [-m %s -c %s app-server]", tool.Args, wantModel, wantCfg)
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

	tool, _, err := m.buildToolCommand(context.Background(), task)
	if err != nil {
		t.Fatalf("buildToolCommand: %v", err)
	}
	wantCfg := `model_reasoning_effort="high"`

	appIdx := indexOfAny(tool.Args, "app-server")
	if appIdx < 0 {
		t.Fatalf("args=%v, expected app-server subcommand", tool.Args)
	}
	if appIdx < 4 {
		t.Fatalf("args=%v, want defaults before app-server", tool.Args)
	}
	if tool.Args[appIdx-4] != "-m" || tool.Args[appIdx-3] != "o3" || tool.Args[appIdx-2] != "-c" || tool.Args[appIdx-1] != wantCfg {
		t.Fatalf("args=%v, want [-m o3 -c %s app-server]", tool.Args, wantCfg)
	}
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
