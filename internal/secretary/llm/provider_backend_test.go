package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/config"
	"controlccx/internal/providers"
)

func TestProviderBackend_RoutesByActiveSecretaryBackend(t *testing.T) {
	var (
		openaiCalls    int32
		anthropicCalls int32
	)

	openaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&openaiCalls, 1)
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OPENAI"}}]}`))
	}))
	t.Cleanup(openaiSrv.Close)

	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&anthropicCalls, 1)
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
			http.Error(w, "missing version", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ANTHROPIC"}]}`))
	}))
	t.Cleanup(anthropicSrv.Close)

	store, err := providers.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}

	openaiProfile, err := store.Upsert(providers.Profile{
		Name: "Sec OpenAI",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{
				Backend: "openai-chat",
				SimpleHTTP: providers.SecretarySimpleHTTP{
					BaseURL:   anthropicSrv.URL,
					AuthToken: "ant-token",
					Model:     "claude-test",
				},
				OpenAIChat: providers.SecretaryOpenAIChat{
					BaseURL: openaiSrv.URL,
					APIKey:  "sk-openai",
					Model:   "gpt-test",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert openai profile: %v", err)
	}
	if _, err := store.Activate("secretary", openaiProfile.ID, nil); err != nil {
		t.Fatalf("activate openai profile: %v", err)
	}

	backend := NewProviderBackendWithProviders(config.Default(), nil, store).(*ProviderBackend)

	out, err := backend.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("complete(openai): %v", err)
	}
	if strings.TrimSpace(out) != "OPENAI" {
		t.Fatalf("out(openai)=%q", out)
	}
	if got := atomic.LoadInt32(&openaiCalls); got != 1 {
		t.Fatalf("openaiCalls=%d want 1", got)
	}
	if got := atomic.LoadInt32(&anthropicCalls); got != 0 {
		t.Fatalf("anthropicCalls=%d want 0", got)
	}
	if strings.TrimSpace(backend.Name()) != "openai-chat" {
		t.Fatalf("backend.Name()=%q", backend.Name())
	}
	if got := strings.TrimSpace(anyToString(backend.LastReceipt()["backend"])); got != "openai-chat" {
		t.Fatalf("receipt.backend=%q", got)
	}

	simpleProfile, err := store.Upsert(providers.Profile{
		Name: "Sec Simple",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{
				Backend: "simple-http",
				SimpleHTTP: providers.SecretarySimpleHTTP{
					BaseURL:   anthropicSrv.URL,
					AuthToken: "ant-token",
					Model:     "claude-test",
				},
				OpenAIChat: providers.SecretaryOpenAIChat{
					BaseURL: openaiSrv.URL,
					APIKey:  "sk-openai",
					Model:   "gpt-test",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert simple profile: %v", err)
	}
	if _, err := store.Activate("secretary", simpleProfile.ID, nil); err != nil {
		t.Fatalf("activate simple profile: %v", err)
	}

	out, err = backend.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("complete(simple): %v", err)
	}
	if strings.TrimSpace(out) != "ANTHROPIC" {
		t.Fatalf("out(simple)=%q", out)
	}
	if got := atomic.LoadInt32(&openaiCalls); got != 1 {
		t.Fatalf("openaiCalls=%d want 1", got)
	}
	if got := atomic.LoadInt32(&anthropicCalls); got != 1 {
		t.Fatalf("anthropicCalls=%d want 1", got)
	}
	if strings.TrimSpace(backend.Name()) != "simple-http" {
		t.Fatalf("backend.Name()=%q", backend.Name())
	}
	if got := strings.TrimSpace(anyToString(backend.LastReceipt()["backend"])); got != "simple-http" {
		t.Fatalf("receipt.backend=%q", got)
	}
}
