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

func TestOpenAIChatBackend_CompleteChat_SendsChatCompletionsRequest(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")

	var gotPath string
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-openai-1")
		_, _ = w.Write([]byte(`{"model":"test-model","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3},"choices":[{"message":{"content":"OK"}}]}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("OPENAI_BASE_URL", srv.URL+"/proxy")

	backend := NewOpenAIChatBackendWithProviders(config.Default(), nil, nil).(*OpenAIChatBackend)
	backend.client = srv.Client()

	temp := 0.2
	maxTokens := 123

	out, err := backend.CompleteChat(context.Background(), []agentsdk.Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "U1"},
		{Role: "assistant", Content: "A1"},
		{Role: "user", Content: "U2"},
	}, &agentsdk.ChatCompletionOptions{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Stop:        []string{"STOP"},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if strings.TrimSpace(out) != "OK" {
		t.Fatalf("out=%q", out)
	}

	if gotPath != "/proxy/v1/chat/completions" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization=%q", gotAuth)
	}

	var parsed struct {
		Model       string   `json:"model"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature *float64 `json:"temperature,omitempty"`
		Stop        []string `json:"stop,omitempty"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
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
	if len(parsed.Stop) != 1 || parsed.Stop[0] != "STOP" {
		t.Fatalf("stop=%v", parsed.Stop)
	}
	if len(parsed.Messages) != 4 {
		t.Fatalf("messages len=%d", len(parsed.Messages))
	}
	if parsed.Messages[0].Role != "system" || parsed.Messages[0].Content != "SYS" {
		t.Fatalf("msg0=%#v", parsed.Messages[0])
	}
	if parsed.Messages[1].Role != "user" || parsed.Messages[1].Content != "U1" {
		t.Fatalf("msg1=%#v", parsed.Messages[1])
	}
	if parsed.Messages[2].Role != "assistant" || parsed.Messages[2].Content != "A1" {
		t.Fatalf("msg2=%#v", parsed.Messages[2])
	}
	if parsed.Messages[3].Role != "user" || parsed.Messages[3].Content != "U2" {
		t.Fatalf("msg3=%#v", parsed.Messages[3])
	}

	receipt := backend.LastReceipt()
	if receipt == nil || len(receipt) == 0 {
		t.Fatalf("expected receipt")
	}
	if receipt["backend"] != "openai-chat" {
		t.Fatalf("receipt.backend=%v", receipt["backend"])
	}
	if receipt["status_code"] != float64(200) && receipt["status_code"] != 200 {
		t.Fatalf("receipt.status_code=%v", receipt["status_code"])
	}
	if receipt["request_id"] != "req-openai-1" {
		t.Fatalf("receipt.request_id=%v", receipt["request_id"])
	}
	if receipt["request_model"] != "test-model" {
		t.Fatalf("receipt.request_model=%v", receipt["request_model"])
	}
	if receipt["model"] != "test-model" {
		t.Fatalf("receipt.model=%v", receipt["model"])
	}
	if _, ok := receipt["usage"]; !ok {
		t.Fatalf("expected receipt.usage, got=%v", receipt)
	}
}

func TestOpenAIChatBackend_CompleteChatStream_CallsCallbackOnce(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OPENAI_BASE_URL", srv.URL)

	backend := NewOpenAIChatBackendWithProviders(config.Default(), nil, nil).(*OpenAIChatBackend)
	backend.client = srv.Client()

	var chunks []string
	err := backend.CompleteChatStream(context.Background(), []agentsdk.Message{{Role: "user", Content: "hi"}}, nil, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(chunks, "") != "OK" {
		t.Fatalf("chunks=%v", chunks)
	}
}
