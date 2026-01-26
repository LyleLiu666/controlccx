package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_ApplyPatch_Persists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	apiKey := "sk-ant-1234567890"
	got, err := store.ApplyPatch(Patch{AnthropicAPIKey: &apiKey})
	if err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if got.AnthropicAPIKey != apiKey {
		t.Fatalf("AnthropicAPIKey=%q, want %q", got.AnthropicAPIKey, apiKey)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var onDisk Secrets
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("json: %v", err)
	}
	if onDisk.AnthropicAPIKey != apiKey {
		t.Fatalf("onDisk.AnthropicAPIKey=%q, want %q", onDisk.AnthropicAPIKey, apiKey)
	}
}

func TestComputeStatus_PrefersEnvOverStored(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key-123456")
	st := ComputeStatus(Secrets{AnthropicAPIKey: "stored-key-abcdef"})
	if st.Claude.APIKey.Effective != "env" {
		t.Fatalf("effective=%q, want env", st.Claude.APIKey.Effective)
	}
}

func TestComputeStatus_UsesStoredWhenEnvMissing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	st := ComputeStatus(Secrets{OpenAIAPIKey: "stored-openai-abcdef"})
	if st.Codex.APIKey.Effective != "stored" {
		t.Fatalf("effective=%q, want stored", st.Codex.APIKey.Effective)
	}
}

