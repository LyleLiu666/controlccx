package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/auth"
	"controlccx/internal/tasks"
)

func TestMergeEnv_StoredOverridesExistingEnv(t *testing.T) {
	base := []string{"ANTHROPIC_API_KEY=envkey"}
	out := mergeEnv(base, map[string]string{"ANTHROPIC_API_KEY": "storedkey"})

	got := ""
	for _, kv := range out {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") {
			got = strings.TrimPrefix(kv, "ANTHROPIC_API_KEY=")
		}
	}
	if got != "storedkey" {
		t.Fatalf("ANTHROPIC_API_KEY=%q, want storedkey", got)
	}
}

func TestManager_envForWorker_InjectsStoredSecrets(t *testing.T) {
	old, had := os.LookupEnv("OPENAI_API_KEY")
	_ = os.Unsetenv("OPENAI_API_KEY")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("OPENAI_API_KEY", old)
		} else {
			_ = os.Unsetenv("OPENAI_API_KEY")
		}
	})

	store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	key := "stored-openai"
	if _, err := store.ApplyPatch(auth.Patch{OpenAIAPIKey: &key}); err != nil {
		t.Fatalf("store.ApplyPatch: %v", err)
	}
	m := &Manager{auth: store}

	out := m.envForWorker(tasks.WorkerCodex)
	found := false
	for _, kv := range out {
		if kv == "OPENAI_API_KEY=stored-openai" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected OPENAI_API_KEY to be injected, env=%v", out)
	}
}

func TestManager_envForWorker_InjectsStoredClaudeConfig(t *testing.T) {
	old, had := os.LookupEnv("ANTHROPIC_BASE_URL")
	_ = os.Unsetenv("ANTHROPIC_BASE_URL")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("ANTHROPIC_BASE_URL", old)
		} else {
			_ = os.Unsetenv("ANTHROPIC_BASE_URL")
		}
	})

	store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	baseURL := "https://open.bigmodel.cn/api/anthropic"
	if _, err := store.ApplyPatch(auth.Patch{AnthropicBaseURL: &baseURL}); err != nil {
		t.Fatalf("store.ApplyPatch: %v", err)
	}
	m := &Manager{auth: store}

	out := m.envForWorker(tasks.WorkerClaudeCode)
	found := false
	for _, kv := range out {
		if kv == "ANTHROPIC_BASE_URL=https://open.bigmodel.cn/api/anthropic" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ANTHROPIC_BASE_URL to be injected, env=%v", out)
	}
}
