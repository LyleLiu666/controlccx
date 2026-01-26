package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/auth"
	"controlccx/internal/tasks"
)

func TestMergeEnv_DoesNotOverrideExisting(t *testing.T) {
	base := []string{"ANTHROPIC_API_KEY=envkey"}
	out := mergeEnv(base, map[string]string{"ANTHROPIC_API_KEY": "storedkey"})

	got := ""
	for _, kv := range out {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") {
			got = strings.TrimPrefix(kv, "ANTHROPIC_API_KEY=")
		}
	}
	if got != "envkey" {
		t.Fatalf("ANTHROPIC_API_KEY=%q, want envkey", got)
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
