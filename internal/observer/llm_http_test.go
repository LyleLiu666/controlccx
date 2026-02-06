package observer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/providers"
)

func TestNormalizeMessagesEndpoint(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{base: "https://api.anthropic.com", want: "https://api.anthropic.com/v1/messages"},
		{base: "https://api.anthropic.com/", want: "https://api.anthropic.com/v1/messages"},
		{base: "https://example.com/proxy", want: "https://example.com/proxy/v1/messages"},
		{base: "https://example.com/proxy/", want: "https://example.com/proxy/v1/messages"},
		{base: "https://example.com/v1/messages", want: "https://example.com/v1/messages"},
	}
	for _, tc := range cases {
		got, err := normalizeMessagesEndpoint(tc.base)
		if err != nil {
			t.Fatalf("base=%q: unexpected err: %v", tc.base, err)
		}
		if got != tc.want {
			t.Fatalf("base=%q: got %q want %q", tc.base, got, tc.want)
		}
	}
}

func TestSimpleHTTPBackend_Complete_AnthropicStyle(t *testing.T) {
	const authToken = "token-abc"
	var gotAuthHeader string
	var gotAPIKeyHeader string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = strings.TrimSpace(r.Header.Get("Authorization"))
		gotAPIKeyHeader = strings.TrimSpace(r.Header.Get("x-api-key"))
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok from anthropic"}]}`))
	}))
	defer srv.Close()

	store, err := auth.Load(t.TempDir() + "/secrets.json")
	if err != nil {
		t.Fatalf("auth load: %v", err)
	}
	_, err = store.ApplyPatch(auth.Patch{
		AnthropicBaseURL:   ptr(srv.URL),
		AnthropicAuthToken: ptr(authToken),
		AnthropicModel:     ptr("claude-test"),
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	cfg := config.Default()
	b := NewSimpleHTTPBackend(cfg, store)
	out, err := b.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "ok from anthropic" {
		t.Fatalf("out=%q", out)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path=%q, want /v1/messages", gotPath)
	}
	if gotAuthHeader != "Bearer "+authToken {
		t.Fatalf("Authorization=%q", gotAuthHeader)
	}
	if gotAPIKeyHeader != authToken {
		t.Fatalf("x-api-key=%q", gotAPIKeyHeader)
	}
}

func TestSimpleHTTPBackend_Complete_OpenAIStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok from choices"}}]}`))
	}))
	defer srv.Close()

	store, err := auth.Load(t.TempDir() + "/secrets.json")
	if err != nil {
		t.Fatalf("auth load: %v", err)
	}
	_, err = store.ApplyPatch(auth.Patch{
		AnthropicBaseURL:   ptr(srv.URL + "/proxy"),
		AnthropicAuthToken: ptr("token-xyz"),
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	cfg := config.Default()
	b := NewSimpleHTTPBackend(cfg, store)
	out, err := b.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "ok from choices" {
		t.Fatalf("out=%q", out)
	}
}

func TestSimpleHTTPBackend_Complete_UsesClaudeLiveConfig(t *testing.T) {
	const authToken = "token-live-123"
	var gotAuthHeader string
	var gotAPIKeyHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = strings.TrimSpace(r.Header.Get("Authorization"))
		gotAPIKeyHeader = strings.TrimSpace(r.Header.Get("x-api-key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok from live"}]}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{
  "env": {
    "ANTHROPIC_BASE_URL": "`+srv.URL+`",
    "ANTHROPIC_AUTH_TOKEN": "`+authToken+`",
    "ANTHROPIC_MODEL": "claude-live-test"
  }
}
`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatalf("auth load: %v", err)
	}
	cfg := config.Default()
	b := NewSimpleHTTPBackend(cfg, store)
	out, err := b.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "ok from live" {
		t.Fatalf("out=%q", out)
	}
	if gotAuthHeader != "Bearer "+authToken {
		t.Fatalf("Authorization=%q", gotAuthHeader)
	}
	if gotAPIKeyHeader != authToken {
		t.Fatalf("x-api-key=%q", gotAPIKeyHeader)
	}
}

func TestSimpleHTTPBackend_Complete_MissingCredential(t *testing.T) {
	store, err := auth.Load(t.TempDir() + "/secrets.json")
	if err != nil {
		t.Fatalf("auth load: %v", err)
	}
	cfg := config.Default()
	b := NewSimpleHTTPBackend(cfg, store)
	_, err = b.Complete(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "missing credentials") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}
}

func TestSimpleHTTPBackend_Complete_UsesSecretaryProviderOverride(t *testing.T) {
	const authToken = "token-from-provider"
	var gotAuthHeader string
	var gotAPIKeyHeader string
	var gotPath string
	var authHits int

	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	srvAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHits++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srvAuth.Close()

	srvProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = strings.TrimSpace(r.Header.Get("Authorization"))
		gotAPIKeyHeader = strings.TrimSpace(r.Header.Get("x-api-key"))
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok from provider"}]}`))
	}))
	defer srvProvider.Close()

	authStore, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatalf("auth load: %v", err)
	}
	_, err = authStore.ApplyPatch(auth.Patch{
		AnthropicBaseURL:   ptr(srvAuth.URL),
		AnthropicAuthToken: ptr("token-from-auth-store"),
		AnthropicModel:     ptr("claude-auth-store"),
	})
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	providersStore, err := providers.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("providers new store: %v", err)
	}
	p, err := providersStore.Upsert(providers.Profile{
		Name: "Secretary",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{
				Backend: "simple-http",
				SimpleHTTP: providers.SecretarySimpleHTTP{
					BaseURL:   srvProvider.URL,
					AuthToken: authToken,
					Model:     "claude-provider-test",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("providers upsert: %v", err)
	}
	if err := providersStore.SetActive("secretary", p.ID); err != nil {
		t.Fatalf("providers set active: %v", err)
	}

	cfg := config.Default()
	b := NewSimpleHTTPBackendWithProviders(cfg, authStore, providersStore)
	out, err := b.Complete(context.Background(), "hello")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if out != "ok from provider" {
		t.Fatalf("out=%q", out)
	}
	if authHits != 0 {
		t.Fatalf("auth store backend should not be used (hits=%d)", authHits)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path=%q, want /v1/messages", gotPath)
	}
	if gotAuthHeader != "Bearer "+authToken {
		t.Fatalf("Authorization=%q", gotAuthHeader)
	}
	if gotAPIKeyHeader != authToken {
		t.Fatalf("x-api-key=%q", gotAPIKeyHeader)
	}
}

func ptr(s string) *string { return &s }
