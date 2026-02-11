package tasks

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_CreateTask_PersistsUnsafeAutomationOption(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:       WorkerClaudeCode,
		Mode:             ModeNew,
		Prompt:           "hi",
		WorkDir:          ".",
		UnsafeAutomation: true,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if !task.UnsafeAutomation {
		t.Fatalf("unsafe_automation=%v, want true", task.UnsafeAutomation)
	}

	loaded, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !loaded.UnsafeAutomation {
		t.Fatalf("loaded unsafe_automation=%v, want true", loaded.UnsafeAutomation)
	}

	list, err := store.ListTasks(ctx, 50)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	found := false
	for _, it := range list {
		if it.ID != task.ID {
			continue
		}
		found = true
		if !it.UnsafeAutomation {
			t.Fatalf("listed unsafe_automation=%v, want true", it.UnsafeAutomation)
		}
	}
	if !found {
		t.Fatalf("task not found in list")
	}
}

func TestStore_CreateTask_PersistsSafetyOptions(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:            WorkerClaudeCode,
		Mode:                  ModeNew,
		Prompt:                "hi",
		WorkDir:               ".",
		SafetyPreset:          "claude:sandboxed-search-browse",
		TaskIntent:            "search-browse",
		ClaudePermissionMode:  "plan",
		ClaudeSandbox:         true,
		ClaudeWebFetchDomains: []string{"docs.claude.com", "github.com"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.SafetyPreset != "claude:sandboxed-search-browse" {
		t.Fatalf("safety_preset=%q, want %q", task.SafetyPreset, "claude:sandboxed-search-browse")
	}
	if task.TaskIntent != "search-browse" {
		t.Fatalf("task_intent=%q, want %q", task.TaskIntent, "search-browse")
	}
	if task.NetworkTier != NetworkTierWebReadonly {
		t.Fatalf("network_tier=%q, want %q", task.NetworkTier, NetworkTierWebReadonly)
	}
	if task.ClaudePermissionMode != "plan" {
		t.Fatalf("claude_permission_mode=%q, want %q", task.ClaudePermissionMode, "plan")
	}
	if !task.ClaudeSandbox {
		t.Fatalf("claude_sandbox=%v, want true", task.ClaudeSandbox)
	}
	if len(task.ClaudeWebFetchDomains) != 2 || task.ClaudeWebFetchDomains[0] == "" || task.ClaudeWebFetchDomains[1] == "" {
		t.Fatalf("claude_webfetch_domains=%v, want non-empty domains", task.ClaudeWebFetchDomains)
	}

	loaded, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loaded.SafetyPreset != task.SafetyPreset {
		t.Fatalf("loaded safety_preset=%q, want %q", loaded.SafetyPreset, task.SafetyPreset)
	}
	if loaded.TaskIntent != task.TaskIntent {
		t.Fatalf("loaded task_intent=%q, want %q", loaded.TaskIntent, task.TaskIntent)
	}
	if loaded.NetworkTier != task.NetworkTier {
		t.Fatalf("loaded network_tier=%q, want %q", loaded.NetworkTier, task.NetworkTier)
	}
	if loaded.ClaudePermissionMode != task.ClaudePermissionMode {
		t.Fatalf("loaded claude_permission_mode=%q, want %q", loaded.ClaudePermissionMode, task.ClaudePermissionMode)
	}
	if loaded.ClaudeSandbox != task.ClaudeSandbox {
		t.Fatalf("loaded claude_sandbox=%v, want %v", loaded.ClaudeSandbox, task.ClaudeSandbox)
	}
	if len(loaded.ClaudeWebFetchDomains) != 2 {
		t.Fatalf("loaded claude_webfetch_domains=%v, want len=2", loaded.ClaudeWebFetchDomains)
	}

	list, err := store.ListTasks(ctx, 50)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	found := false
	for _, it := range list {
		if it.ID != task.ID {
			continue
		}
		found = true
		if it.SafetyPreset != task.SafetyPreset {
			t.Fatalf("listed safety_preset=%q, want %q", it.SafetyPreset, task.SafetyPreset)
		}
		if it.TaskIntent != task.TaskIntent {
			t.Fatalf("listed task_intent=%q, want %q", it.TaskIntent, task.TaskIntent)
		}
		if it.NetworkTier != task.NetworkTier {
			t.Fatalf("listed network_tier=%q, want %q", it.NetworkTier, task.NetworkTier)
		}
		if it.ClaudePermissionMode != task.ClaudePermissionMode {
			t.Fatalf("listed claude_permission_mode=%q, want %q", it.ClaudePermissionMode, task.ClaudePermissionMode)
		}
		if it.ClaudeSandbox != task.ClaudeSandbox {
			t.Fatalf("listed claude_sandbox=%v, want %v", it.ClaudeSandbox, task.ClaudeSandbox)
		}
	}
	if !found {
		t.Fatalf("task not found in list")
	}
}

func TestStore_CreateTask_PersistsExplicitNetworkTier(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	task, err := store.CreateTask(ctx, CreateTaskInput{
		WorkerType:  WorkerCodex,
		Mode:        ModeNew,
		Prompt:      "hi",
		WorkDir:     ".",
		NetworkTier: NetworkTierOff,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.NetworkTier != NetworkTierOff {
		t.Fatalf("network_tier=%q, want %q", task.NetworkTier, NetworkTierOff)
	}
}
