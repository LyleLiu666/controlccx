package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestComputeStatus_PrefersStoredOverEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key-123456")
	st := ComputeStatus(Secrets{AnthropicAPIKey: "stored-key-abcdef"})
	if st.Claude.APIKey.Effective != "stored" {
		t.Fatalf("effective=%q, want stored", st.Claude.APIKey.Effective)
	}
}

func TestComputeStatus_ClaudeAuthToken_EnablesAvailability(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "env-token-123456")
	st := ComputeStatus(Secrets{})
	if st.Claude.AuthToken.Effective != "env" {
		t.Fatalf("effective=%q, want env", st.Claude.AuthToken.Effective)
	}
	if !st.Claude.Available {
		t.Fatalf("expected claude available with ANTHROPIC_AUTH_TOKEN")
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
	if st.Codex.APIKey.Effective != "live" {
		t.Fatalf("effective=%q, want live", st.Codex.APIKey.Effective)
	}
	if !st.Codex.Available {
		t.Fatalf("expected codex available")
	}
}

func TestComputeStatus_ClaudeAuth_UsesClaudeSettingsJSON(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-ant-live-abcdef"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := ComputeStatus(Secrets{})
	if st.Claude.AuthToken.Effective != "live" {
		t.Fatalf("effective=%q, want live", st.Claude.AuthToken.Effective)
	}
	if st.Claude.AuthToken.Masked != MaskSecret("sk-ant-live-abcdef") {
		t.Fatalf("masked=%q", st.Claude.AuthToken.Masked)
	}
	if !st.Claude.Available {
		t.Fatalf("expected claude available")
	}
}

func TestComputeStatus_WarnsOnEnvOverrides(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-ant-env-abcdef")

	st := ComputeStatus(Secrets{AnthropicAuthToken: "sk-ant-stored"})
	if st.Claude.AuthToken.Effective != "stored" {
		t.Fatalf("effective=%q, want stored", st.Claude.AuthToken.Effective)
	}
	if len(st.Warnings) == 0 {
		t.Fatalf("expected warnings")
	}
	found := false
	for _, w := range st.Warnings {
		if strings.Contains(w, "ANTHROPIC_AUTH_TOKEN") && strings.Contains(w, "可导入") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected import hint mentioning ANTHROPIC_AUTH_TOKEN, got=%v", st.Warnings)
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
