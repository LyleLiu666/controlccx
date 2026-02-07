package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/chat"
	"controlccx/internal/db"
)

func TestSecretaryChat_HistoryLog_WritesFullMessageList(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	chatStore := chat.NewStore(conn)
	responder := &stubChatResponder{reply: "pong"}

	// Pre-existing history.
	if _, err := chatStore.Append(ctx, chat.RoleAssistant, "hello"); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	if _, err := chatStore.Append(ctx, chat.RoleUser, "hi"); err != nil {
		t.Fatalf("append user: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "secretary", "chat_history.jsonl")
	logger := newSecretaryChatHistoryLogger(logPath)
	if logger == nil {
		t.Fatalf("expected history logger")
	}

	handler := newSecretaryChatHandler(secretaryChatHandlerDeps{
		Chat:        chatStore,
		Responder:   responder,
		Idempotency: newChatIdempotencyCache(2*time.Second, 128),
		HistoryLog:  logger,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

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

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines=%d, want 1", len(lines))
	}
	var entry secretaryChatHistoryLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Kind != "llm_request" {
		t.Fatalf("kind=%q, want llm_request", entry.Kind)
	}
	if entry.IdempotencyKey != "chat-key-1" {
		t.Fatalf("idempotency_key=%q, want chat-key-1", entry.IdempotencyKey)
	}
	if entry.UserMessage != "ping" {
		t.Fatalf("user_message=%q, want ping", entry.UserMessage)
	}
	if len(entry.Messages) != 3 {
		t.Fatalf("messages=%d, want 3", len(entry.Messages))
	}
	last := entry.Messages[len(entry.Messages)-1]
	if last.Role != chat.RoleUser || last.Content != "ping" {
		t.Fatalf("last message=%+v, want user ping", last)
	}
}
