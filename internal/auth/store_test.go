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

	baseURL := "https://open.bigmodel.cn/api/anthropic"
	apiKey := "sk-ant-1234567890"
	got, err := store.ApplyPatch(Patch{AnthropicBaseURL: &baseURL, AnthropicAPIKey: &apiKey})
	if err != nil {
		t.Fatalf("ApplyPatch: %v", err)
	}
	if got.AnthropicBaseURL != baseURL {
		t.Fatalf("AnthropicBaseURL=%q, want %q", got.AnthropicBaseURL, baseURL)
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
	if onDisk.AnthropicBaseURL != baseURL {
		t.Fatalf("onDisk.AnthropicBaseURL=%q, want %q", onDisk.AnthropicBaseURL, baseURL)
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

func TestComputeStatus_BaseURL_UsesStoredWhenEnvMissing(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "")
	url := "https://open.bigmodel.cn/api/anthropic"
	st := ComputeStatus(Secrets{AnthropicBaseURL: url})
	if st.Claude.BaseURL.Effective != "stored" {
		t.Fatalf("effective=%q, want stored", st.Claude.BaseURL.Effective)
	}
	if st.Claude.BaseURL.Masked != url {
		t.Fatalf("base_url=%q, want %q", st.Claude.BaseURL.Masked, url)
	}
}

func TestComputeStatus_CodexAuth_UsesCodexHomeAuthJSON(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := ComputeStatus(Secrets{})
	if st.Codex.APIKey.Effective != "codex" {
		t.Fatalf("effective=%q, want codex", st.Codex.APIKey.Effective)
	}
	if !st.Codex.Available {
		t.Fatalf("expected codex available")
	}
}

func TestComputeStatus_CodexModel_Defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_HOME", t.TempDir())
	st := ComputeStatus(Secrets{})
	if st.Codex.Model.Effective != "default" || st.Codex.Model.Masked != "gpt-5.2" {
		t.Fatalf("model=%+v, want default gpt-5.2", st.Codex.Model)
	}
	if st.Codex.ReasoningEffort.Effective != "default" || st.Codex.ReasoningEffort.Masked != "xhigh" {
		t.Fatalf("effort=%+v, want default xhigh", st.Codex.ReasoningEffort)
	}
}

func TestComputeStatus_CodexModel_UsesStored(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_HOME", t.TempDir())
	st := ComputeStatus(Secrets{CodexModel: "o3", CodexReasoningEffort: "high"})
	if st.Codex.Model.Effective != "stored" || st.Codex.Model.Masked != "o3" {
		t.Fatalf("model=%+v, want stored o3", st.Codex.Model)
	}
	if st.Codex.ReasoningEffort.Effective != "stored" || st.Codex.ReasoningEffort.Masked != "high" {
		t.Fatalf("effort=%+v, want stored high", st.Codex.ReasoningEffort)
	}
}
