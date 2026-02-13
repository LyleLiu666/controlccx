package llm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/config"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSecretaryLLMTimeout_EnvOverridesConfig(t *testing.T) {
	t.Setenv("CONTROLCCX_SECRETARY_LLM_TIMEOUT", "500ms")

	// Simple HTTP credentials.
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	// OpenAI credentials.
	t.Setenv("OPENAI_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Secretary.LLMTimeout = "1ms"

	var (
		gotSimple time.Duration
		gotOpenAI time.Duration
	)

	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatalf("expected request context deadline to be set")
		}
		remaining := time.Until(deadline)
		if strings.Contains(r.URL.Path, "/v1/messages") {
			gotSimple = remaining
			body := io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"OK"}]}`))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    r,
			}, nil
		}
		if strings.Contains(r.URL.Path, "/v1/chat/completions") {
			gotOpenAI = remaining
			body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"}}]}`))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    r,
			}, nil
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
		return nil, nil
	})
	client := &http.Client{Transport: rt}

	simple := NewSimpleHTTPBackendWithProviders(cfg, nil, nil).(*SimpleHTTPBackend)
	simple.client = client

	openai := NewOpenAIChatBackendWithProviders(cfg, nil, nil).(*OpenAIChatBackend)
	openai.client = client

	if _, err := simple.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("simple-http complete: %v", err)
	}
	if _, err := openai.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("openai-chat complete: %v", err)
	}

	// Remaining time should be close to 500ms at RoundTrip time (allow some slack).
	if gotSimple <= 100*time.Millisecond || gotSimple > 500*time.Millisecond {
		t.Fatalf("simple-http remaining=%s want ~500ms", gotSimple)
	}
	if gotOpenAI <= 100*time.Millisecond || gotOpenAI > 500*time.Millisecond {
		t.Fatalf("openai-chat remaining=%s want ~500ms", gotOpenAI)
	}
}

func TestSecretaryLLMTimeout_ConfigAppliesWhenEnvMissing(t *testing.T) {
	t.Setenv("CONTROLCCX_SECRETARY_LLM_TIMEOUT", "")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Secretary.LLMTimeout = "250ms"

	var got time.Duration
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatalf("expected request context deadline to be set")
		}
		got = time.Until(deadline)
		body := io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"OK"}]}`))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
			Request:    r,
		}, nil
	})

	backend := NewSimpleHTTPBackendWithProviders(cfg, nil, nil).(*SimpleHTTPBackend)
	backend.client = &http.Client{Transport: rt}

	if _, err := backend.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got <= 20*time.Millisecond || got > 250*time.Millisecond {
		t.Fatalf("remaining=%s want ~250ms", got)
	}
}

func TestSecretaryLLMTimeout_DisabledByZero(t *testing.T) {
	t.Setenv("CONTROLCCX_SECRETARY_LLM_TIMEOUT", "0")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Secretary.LLMTimeout = "150ms"

	var sawDeadline bool
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		_, ok := r.Context().Deadline()
		sawDeadline = ok
		body := io.NopCloser(bytes.NewReader([]byte(`{"content":[{"type":"text","text":"OK"}]}`)))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
			Request:    r,
		}, nil
	})

	backend := NewSimpleHTTPBackendWithProviders(cfg, nil, nil).(*SimpleHTTPBackend)
	backend.client = &http.Client{Transport: rt}

	if _, err := backend.CompleteChat(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if sawDeadline {
		t.Fatalf("expected no deadline when timeout is disabled")
	}
}
