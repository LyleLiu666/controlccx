package providers

import (
	"path/filepath"
	"testing"

	"controlccx/internal/auth"
)

func TestStoreActivate_ClaudeUpdatesSecretsAndActive(t *testing.T) {
	dir := t.TempDir()
	providersStore, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	if _, err := authStore.ApplyPatch(auth.Patch{
		OpenAIAPIKey: ptr("sk-openai-old"),
	}); err != nil {
		t.Fatalf("seed secrets: %v", err)
	}

	p, err := providersStore.Upsert(Profile{
		Name: "P1",
		Targets: Targets{
			Claude: ClaudeTarget{
				BaseURL:   "https://example.com/api/anthropic",
				AuthToken: "sk-ant-test",
				Model:     "claude-3-7-sonnet",
			},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := providersStore.Activate("claude", p.ID, authStore); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got := providersStore.Active().Claude; got != p.ID {
		t.Fatalf("active claude got=%q want=%q", got, p.ID)
	}

	secrets := authStore.Get()
	if secrets.AnthropicBaseURL != "https://example.com/api/anthropic" {
		t.Fatalf("base url got=%q", secrets.AnthropicBaseURL)
	}
	if secrets.AnthropicAuthToken != "sk-ant-test" {
		t.Fatalf("auth token got=%q", secrets.AnthropicAuthToken)
	}
	if secrets.AnthropicModel != "claude-3-7-sonnet" {
		t.Fatalf("model got=%q", secrets.AnthropicModel)
	}
	if secrets.AnthropicAPIKey != "" {
		t.Fatalf("expected api key to be cleared, got=%q", secrets.AnthropicAPIKey)
	}
	if secrets.OpenAIAPIKey != "sk-openai-old" {
		t.Fatalf("expected codex secret untouched, got=%q", secrets.OpenAIAPIKey)
	}
}

func TestStoreActivate_CodexUpdatesSecretsAndActive(t *testing.T) {
	dir := t.TempDir()
	providersStore, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	if _, err := authStore.ApplyPatch(auth.Patch{
		AnthropicAuthToken: ptr("sk-ant-old"),
	}); err != nil {
		t.Fatalf("seed secrets: %v", err)
	}

	p, err := providersStore.Upsert(Profile{
		Name: "P2",
		Targets: Targets{
			Codex: CodexTarget{
				APIKey:          "sk-openai-test",
				Model:           "gpt-5.3-codex",
				ReasoningEffort: "xhigh",
			},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := providersStore.Activate("codex", p.ID, authStore); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got := providersStore.Active().Codex; got != p.ID {
		t.Fatalf("active codex got=%q want=%q", got, p.ID)
	}

	secrets := authStore.Get()
	if secrets.OpenAIAPIKey != "sk-openai-test" {
		t.Fatalf("openai api key got=%q", secrets.OpenAIAPIKey)
	}
	if secrets.CodexModel != "gpt-5.3-codex" {
		t.Fatalf("model got=%q", secrets.CodexModel)
	}
	if secrets.CodexReasoningEffort != "xhigh" {
		t.Fatalf("effort got=%q", secrets.CodexReasoningEffort)
	}
	if secrets.AnthropicAuthToken != "sk-ant-old" {
		t.Fatalf("expected anthropic secret untouched, got=%q", secrets.AnthropicAuthToken)
	}
}

func TestStoreActivate_SecretaryOnlyUpdatesActive(t *testing.T) {
	dir := t.TempDir()
	providersStore, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p, err := providersStore.Upsert(Profile{Name: "P3", Targets: Targets{Secretary: SecretaryTarget{Backend: "auto"}}})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := providersStore.Activate("secretary", p.ID, nil); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got := providersStore.Active().Secretary; got != p.ID {
		t.Fatalf("active secretary got=%q want=%q", got, p.ID)
	}
}

func TestStoreActivate_ValidatesInputs(t *testing.T) {
	dir := t.TempDir()
	providersStore, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p, err := providersStore.Upsert(Profile{Name: "P1"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := providersStore.Activate("unknown", p.ID, nil); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := providersStore.Activate("claude", "missing", &auth.Store{}); err == nil {
		t.Fatalf("expected missing profile error")
	}
	if _, err := providersStore.Activate("claude", p.ID, nil); err == nil {
		t.Fatalf("expected auth store required error")
	}
}

func ptr(s string) *string { return &s }
