package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/config"
)

func TestSimpleHTTPBackend_CompleteChat_SendsStructuredMessagesWithPromptCaching(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
			t.Fatalf("missing anthropic-version header")
		}
		if v := strings.TrimSpace(r.Header.Get("anthropic-beta")); v != "prompt-caching" {
			t.Fatalf("anthropic-beta=%q", v)
		}
		if v := strings.TrimSpace(r.Header.Get("x-api-key")); v != "test-key" {
			t.Fatalf("x-api-key=%q", v)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = b

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"OK"}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	backend := NewSimpleHTTPBackendWithProviders(config.Default(), nil, nil).(*SimpleHTTPBackend)
	backend.client = srv.Client()

	temp := 0.1
	maxTokens := 123

	out, err := backend.CompleteChat(context.Background(), []agentsdk.Message{
		{Role: "system", Content: "SYS-1"},
		{Role: "system", Content: "SYS-2"},
		{Role: "user", Content: "U-1"},
		{Role: "assistant", Content: "A-1"},
		{Role: "user", Content: "U-2"},
	}, &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
		CacheEpoch:        7,
		Temperature:       &temp,
		MaxTokens:         &maxTokens,
		Stop:              []string{"STOP"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if strings.TrimSpace(out) != "OK" {
		t.Fatalf("out=%q", out)
	}

	type cacheControl struct {
		Type string `json:"type"`
	}
	type contentBlock struct {
		Type         string        `json:"type"`
		Text         string        `json:"text"`
		CacheControl *cacheControl `json:"cache_control,omitempty"`
	}
	type msg struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}
	type req struct {
		Model         string         `json:"model"`
		MaxTokens     int            `json:"max_tokens"`
		System        []contentBlock `json:"system"`
		Messages      []msg          `json:"messages"`
		Temperature   *float64       `json:"temperature,omitempty"`
		StopSequences []string       `json:"stop_sequences,omitempty"`
	}

	var parsed req
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if parsed.Model != "test-model" {
		t.Fatalf("model=%q", parsed.Model)
	}
	if parsed.MaxTokens != 123 {
		t.Fatalf("max_tokens=%d", parsed.MaxTokens)
	}
	if parsed.Temperature == nil || *parsed.Temperature != temp {
		t.Fatalf("temperature=%v want %v", parsed.Temperature, temp)
	}
	if len(parsed.StopSequences) != 1 || parsed.StopSequences[0] != "STOP" {
		t.Fatalf("stop_sequences=%v", parsed.StopSequences)
	}

	// System messages are passed via the top-level system field.
	if len(parsed.System) < 2 {
		t.Fatalf("system len=%d", len(parsed.System))
	}
	var (
		sys1Idx = -1
		sys2Idx = -1
	)
	for i, blk := range parsed.System {
		switch blk.Text {
		case "SYS-1":
			sys1Idx = i
		case "SYS-2":
			sys2Idx = i
		}
	}
	if sys1Idx == -1 || sys2Idx == -1 {
		t.Fatalf("expected SYS-1 and SYS-2 in system blocks, got %#v", parsed.System)
	}
	if parsed.System[sys1Idx].CacheControl == nil || parsed.System[sys1Idx].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control on SYS-1, got %#v", parsed.System[sys1Idx].CacheControl)
	}
	if parsed.System[sys2Idx].CacheControl == nil || parsed.System[sys2Idx].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control on SYS-2, got %#v", parsed.System[sys2Idx].CacheControl)
	}

	if len(parsed.Messages) != 3 {
		t.Fatalf("messages len=%d want 3", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "user" || parsed.Messages[0].Content[0].Text != "U-1" {
		t.Fatalf("msg0=%#v", parsed.Messages[0])
	}
	if parsed.Messages[1].Role != "assistant" || parsed.Messages[1].Content[0].Text != "A-1" {
		t.Fatalf("msg1=%#v", parsed.Messages[1])
	}
	if parsed.Messages[2].Role != "user" || parsed.Messages[2].Content[0].Text != "U-2" {
		t.Fatalf("msg2=%#v", parsed.Messages[2])
	}

	// Prompt caching marks the most recent 2 messages.
	if parsed.Messages[1].Content[0].CacheControl == nil || parsed.Messages[1].Content[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control on assistant tail, got %#v", parsed.Messages[1].Content[0].CacheControl)
	}
	if parsed.Messages[2].Content[0].CacheControl == nil || parsed.Messages[2].Content[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control on user tail, got %#v", parsed.Messages[2].Content[0].CacheControl)
	}
}

type chatBackendStub struct {
	calls int
}

func (b *chatBackendStub) Name() string { return "stub" }

func (b *chatBackendStub) Complete(ctx context.Context, prompt string) (string, error) {
	t := strings.TrimSpace(prompt)
	if t != "" {
		return "", nil
	}
	return "", nil
}

func (b *chatBackendStub) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	_ = ctx
	_ = messages
	_ = opts
	b.calls++
	return "ok", nil
}

func TestClient_PrefersChatBackendWhenAvailable(t *testing.T) {
	stub := &chatBackendStub{}
	c := &Client{Backend: stub}
	err := c.ChatCompletionStream(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, &agentsdk.ChatCompletionOptions{}, func(chunk string) error {
		if strings.TrimSpace(chunk) != "ok" {
			t.Fatalf("chunk=%q", chunk)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("calls=%d want 1", stub.calls)
	}
}
