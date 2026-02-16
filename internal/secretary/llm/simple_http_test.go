package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/config"
)

func TestSimpleHTTPBackend_CompleteChat_SendsStructuredMessagesWithPromptCaching(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	var (
		mu      sync.Mutex
		gotBody []byte
		reqErr  error
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordErr := func(err error) {
			if err == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if reqErr != nil {
				return
			}
			reqErr = err
		}

		if r.Method != http.MethodPost {
			recordErr(fmt.Errorf("method=%s", r.Method))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/v1/messages" {
			recordErr(fmt.Errorf("path=%s", r.URL.Path))
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
			recordErr(fmt.Errorf("missing anthropic-version header"))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if v := strings.TrimSpace(r.Header.Get("anthropic-beta")); v != "prompt-caching" {
			recordErr(fmt.Errorf("anthropic-beta=%q", v))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if v := strings.TrimSpace(r.Header.Get("x-api-key")); v != "test-key" {
			recordErr(fmt.Errorf("x-api-key=%q", v))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			recordErr(err)
			http.Error(w, "read body failed", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		gotBody = b
		mu.Unlock()

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
	mu.Lock()
	seenErr := reqErr
	body := append([]byte(nil), gotBody...)
	mu.Unlock()
	if seenErr != nil {
		t.Fatalf("server request check failed: %v", seenErr)
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
	if err := json.Unmarshal(body, &parsed); err != nil {
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

	// Prompt caching marks one stable tail anchor and skips newest user input.
	if parsed.Messages[1].Content[0].CacheControl == nil || parsed.Messages[1].Content[0].CacheControl.Type != "ephemeral" {
		t.Fatalf("expected cache_control on assistant tail, got %#v", parsed.Messages[1].Content[0].CacheControl)
	}
	if parsed.Messages[2].Content[0].CacheControl != nil {
		t.Fatalf("expected newest user input to stay uncached, got %#v", parsed.Messages[2].Content[0].CacheControl)
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

type chatStreamBackendStub struct {
	chatCalls   int
	streamCalls int
}

func (b *chatStreamBackendStub) Name() string { return "stream-stub" }

func (b *chatStreamBackendStub) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "ignored", nil
}

func (b *chatStreamBackendStub) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	_ = ctx
	_ = messages
	_ = opts
	b.chatCalls++
	return "fallback", nil
}

func (b *chatStreamBackendStub) CompleteChatStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = messages
	_ = opts
	b.streamCalls++
	if err := callback("A"); err != nil {
		return err
	}
	return callback("B")
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

func TestClient_PrefersChatStreamBackendWhenAvailable(t *testing.T) {
	stub := &chatStreamBackendStub{}
	c := &Client{Backend: stub}
	var chunks []string
	err := c.ChatCompletionStream(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, &agentsdk.ChatCompletionOptions{}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if stub.streamCalls != 1 {
		t.Fatalf("stream_calls=%d want 1", stub.streamCalls)
	}
	if stub.chatCalls != 0 {
		t.Fatalf("chat_calls=%d want 0", stub.chatCalls)
	}
	if strings.Join(chunks, "") != "AB" {
		t.Fatalf("chunks=%v want [A B]", chunks)
	}
}

func TestSimpleHTTPBackend_LastReceipt_IncludesUsageAndKVCache(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "req-abc")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"usage":{
				"input_tokens":100,
				"output_tokens":20,
				"cache_read_input_tokens":80,
				"cache_creation_input_tokens":12
			},
			"content":[{"type":"text","text":"OK"}]
		}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	backend := NewSimpleHTTPBackendWithProviders(config.Default(), nil, nil).(*SimpleHTTPBackend)
	backend.client = srv.Client()

	out, err := backend.CompleteChat(context.Background(), []agentsdk.Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "hi"},
	}, &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
		CacheEpoch:        9,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if strings.TrimSpace(out) != "OK" {
		t.Fatalf("out=%q", out)
	}

	receipt := backend.LastReceipt()
	if strings.TrimSpace(anyToString(receipt["request_id"])) != "req-abc" {
		t.Fatalf("request_id=%v", receipt["request_id"])
	}
	if strings.TrimSpace(anyToString(receipt["model"])) == "" {
		t.Fatalf("expected model in receipt, got=%v", receipt["model"])
	}

	usage, _ := receipt["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("expected usage map in receipt, got=%T", receipt["usage"])
	}
	if got := int(anyToFloat(usage["cache_read_input_tokens"])); got != 80 {
		t.Fatalf("cache_read_input_tokens=%d want 80", got)
	}

	kv, _ := receipt["kv_cache"].(map[string]any)
	if kv == nil {
		t.Fatalf("expected kv_cache map in receipt, got=%T", receipt["kv_cache"])
	}
	if got := int(anyToFloat(kv["cache_read_input_tokens"])); got != 80 {
		t.Fatalf("kv.cache_read_input_tokens=%d want 80", got)
	}
	if got := int(anyToFloat(receipt["request_cache_epoch"])); got != 9 {
		t.Fatalf("request_cache_epoch=%d want 9", got)
	}
}

func TestSimpleHTTPBackend_CompleteChatStream_StreamsDeltasAndStoresReceipt(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	gotStream := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if v, _ := req["stream"].(bool); v {
			gotStream = true
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("request-id", "req-stream-1")
		flusher, _ := w.(http.Flusher)

		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-stream\",\"usage\":{\"input_tokens\":42,\"cache_read_input_tokens\":30}}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	backend := NewSimpleHTTPBackendWithProviders(config.Default(), nil, nil).(*SimpleHTTPBackend)
	backend.client = srv.Client()

	var chunks []string
	err := backend.CompleteChatStream(context.Background(), []agentsdk.Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "hi"},
	}, &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
		CacheEpoch:        3,
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteChatStream: %v", err)
	}
	if !gotStream {
		t.Fatalf("expected request stream=true")
	}
	if got := strings.Join(chunks, ""); got != "Hello" {
		t.Fatalf("chunks join=%q want %q", got, "Hello")
	}

	receipt := backend.LastReceipt()
	if strings.TrimSpace(anyToString(receipt["request_id"])) != "req-stream-1" {
		t.Fatalf("request_id=%v", receipt["request_id"])
	}
	if strings.TrimSpace(anyToString(receipt["model"])) != "claude-stream" {
		t.Fatalf("model=%v", receipt["model"])
	}
	usage, _ := receipt["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("usage missing in receipt")
	}
	if got := int(anyToFloat(usage["output_tokens"])); got != 5 {
		t.Fatalf("usage.output_tokens=%d want 5", got)
	}
	kv, _ := receipt["kv_cache"].(map[string]any)
	if kv == nil {
		t.Fatalf("kv_cache missing in receipt")
	}
	if got := int(anyToFloat(kv["cache_read_input_tokens"])); got != 30 {
		t.Fatalf("kv.cache_read_input_tokens=%d want 30", got)
	}
}

func TestSimpleHTTPBackend_CompleteChatStream_PreservesWhitespaceDeltas(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" \"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"world\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	backend := NewSimpleHTTPBackendWithProviders(config.Default(), nil, nil).(*SimpleHTTPBackend)
	backend.client = srv.Client()

	var chunks []string
	err := backend.CompleteChatStream(context.Background(), []agentsdk.Message{
		{Role: "user", Content: "hi"},
	}, &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteChatStream: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "Hello world" {
		t.Fatalf("chunks join=%q want %q", got, "Hello world")
	}
}

func TestSimpleHTTPBackend_CompleteChatStream_FallsBackToJSONWhenProviderNoSSE(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_MODEL", "test-model")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "req-json-fallback")
		_, _ = io.WriteString(w, `{"model":"claude-json","usage":{"input_tokens":11,"output_tokens":3},"content":[{"type":"text","text":"OK"}]}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	backend := NewSimpleHTTPBackendWithProviders(config.Default(), nil, nil).(*SimpleHTTPBackend)
	backend.client = srv.Client()

	var chunks []string
	err := backend.CompleteChatStream(context.Background(), []agentsdk.Message{
		{Role: "user", Content: "hi"},
	}, &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteChatStream fallback: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "OK" {
		t.Fatalf("chunks join=%q want %q", got, "OK")
	}

	receipt := backend.LastReceipt()
	if strings.TrimSpace(anyToString(receipt["request_id"])) != "req-json-fallback" {
		t.Fatalf("request_id=%v", receipt["request_id"])
	}
	if strings.TrimSpace(anyToString(receipt["model"])) != "claude-json" {
		t.Fatalf("model=%v", receipt["model"])
	}
}

type unexpectedEOFReader struct {
	data []byte
}

func (r *unexpectedEOFReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestParseAnthropicStreamResponse_TreatsUnexpectedEOFAsEOF(t *testing.T) {
	body := &unexpectedEOFReader{
		data: []byte(
			"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}",
		),
	}

	var chunks []string
	text, _, _, err := parseAnthropicStreamResponse(body, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "Hello" {
		t.Fatalf("chunks join=%q want %q", got, "Hello")
	}
	if got := strings.TrimSpace(text); got != "Hello" {
		t.Fatalf("text=%q want %q", got, "Hello")
	}
}

func anyToString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func anyToFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}
