package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
)

type stubChatResponder struct {
	mu    sync.Mutex
	calls int
	delay time.Duration
	reply string
}

func (s *stubChatResponder) RespondWithOptions(ctx context.Context, userMessage string, opts observer.RespondOptions) (observer.Reply, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return observer.Reply{}, ctx.Err()
		case <-timer.C:
		}
	}
	return observer.Reply{Message: s.reply}, nil
}

func (s *stubChatResponder) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestSecretaryChat_IdempotencyKey_DedupesConcurrentSends(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	chatStore := chat.NewStore(conn)
	hub := events.NewHub()
	responder := &stubChatResponder{delay: 80 * time.Millisecond, reply: "pong"}

	handler := newSecretaryChatHandler(secretaryChatHandlerDeps{
		Chat:        chatStore,
		Hub:         hub,
		Responder:   responder,
		Idempotency: newChatIdempotencyCache(2*time.Second, 128),
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	doSend := func() string {
		reqBody, _ := json.Marshal(map[string]any{
			"message": "ping",
		})
		req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "chat-key-1")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var out struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Message
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var a, b string
	go func() {
		defer wg.Done()
		a = doSend()
	}()
	go func() {
		defer wg.Done()
		b = doSend()
	}()
	wg.Wait()

	if a != "pong" || b != "pong" {
		t.Fatalf("unexpected replies: a=%q b=%q", a, b)
	}
	if got := responder.CallCount(); got != 1 {
		t.Fatalf("responder calls=%d, want 1", got)
	}

	msgs, err := chatStore.List(ctx, 0, 10)
	if err != nil {
		t.Fatalf("chat list: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("messages=%d, want 2", len(msgs))
	}
	if msgs[0].Role != chat.RoleUser || msgs[1].Role != chat.RoleAssistant {
		t.Fatalf("unexpected roles: %+v", msgs)
	}
}
