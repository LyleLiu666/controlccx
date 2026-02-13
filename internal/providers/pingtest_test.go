package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPingTest_SucceedsOnAnthropicCompatibleServer(t *testing.T) {
	var gotAuth string
	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("anthropic-version")

		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Model == "" || body.MaxTokens <= 0 || len(body.Messages) == 0 {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if !strings.Contains(strings.ToLower(body.Messages[0].Content), "ping") {
			http.Error(w, "expected ping prompt", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
	}))
	t.Cleanup(srv.Close)

	res := PingTest(context.Background(), SecretarySimpleHTTP{
		BaseURL:   srv.URL,
		AuthToken: "sec-token-abc123",
		Model:     "claude-test",
	}, PingTestOptions{Timeout: 500 * time.Millisecond})
	if !res.OK {
		t.Fatalf("expected ok, got=%+v", res)
	}
	if strings.TrimSpace(res.Response) != "pong" {
		t.Fatalf("response=%q", res.Response)
	}
	if gotAuth != "Bearer sec-token-abc123" {
		t.Fatalf("authorization=%q", gotAuth)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version=%q", gotVersion)
	}
}

func TestPingTestOpenAIChat_SucceedsOnChatCompletionsServer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")

		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Model == "" || body.MaxTokens <= 0 || len(body.Messages) == 0 {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		if !strings.Contains(strings.ToLower(body.Messages[0].Content), "ping") {
			http.Error(w, "expected ping prompt", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	t.Cleanup(srv.Close)

	res := PingTestOpenAIChat(context.Background(), SecretaryOpenAIChat{
		BaseURL: srv.URL,
		APIKey:  "sk-openai-test-abc123",
		Model:   "gpt-test",
	}, PingTestOptions{Timeout: 500 * time.Millisecond})
	if !res.OK {
		t.Fatalf("expected ok, got=%+v", res)
	}
	if strings.TrimSpace(res.Response) != "pong" {
		t.Fatalf("response=%q", res.Response)
	}
	if gotAuth != "Bearer sk-openai-test-abc123" {
		t.Fatalf("authorization=%q", gotAuth)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPingTestOpenAIChat_DefaultBaseURL_WhenEmpty(t *testing.T) {
	var gotURL string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			body := io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"pong"}}]}`))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		}),
	}

	res := PingTestOpenAIChat(context.Background(), SecretaryOpenAIChat{
		APIKey: "sk-openai-test",
		Model:  "gpt-test",
	}, PingTestOptions{Client: client, Timeout: 500 * time.Millisecond})
	if !res.OK {
		t.Fatalf("expected ok, got=%+v", res)
	}
	if strings.TrimSpace(res.Response) != "pong" {
		t.Fatalf("response=%q", res.Response)
	}
	if gotURL != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("url=%q", gotURL)
	}
}

func TestPingTest_MissingCredentials(t *testing.T) {
	res := PingTest(context.Background(), SecretarySimpleHTTP{BaseURL: "https://api.anthropic.com"}, PingTestOptions{})
	if res.OK {
		t.Fatalf("expected not ok, got=%+v", res)
	}
	if res.Hint != "missing_credentials" {
		t.Fatalf("hint=%q", res.Hint)
	}
	if res.Error == "" {
		t.Fatalf("expected error")
	}
}

func TestPingTest_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
	}))
	t.Cleanup(srv.Close)

	res := PingTest(context.Background(), SecretarySimpleHTTP{
		BaseURL:   srv.URL,
		AuthToken: "sec-token-abc123",
		Model:     "claude-test",
	}, PingTestOptions{Timeout: 20 * time.Millisecond})
	if res.OK {
		t.Fatalf("expected not ok, got=%+v", res)
	}
	if res.Hint != "timeout" {
		t.Fatalf("hint=%q", res.Hint)
	}
	if res.Error == "" {
		t.Fatalf("expected error")
	}
}
