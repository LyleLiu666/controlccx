package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"controlccx/internal/chat"
)

type secretaryChatHistoryLogEntry struct {
	Time           time.Time      `json:"time"`
	Kind           string         `json:"kind"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Backend        string         `json:"backend,omitempty"`
	MaxSteps       int            `json:"max_steps,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
	UserMessage    string         `json:"user_message,omitempty"`
	Cached         bool           `json:"cached,omitempty"`
	Reply          string         `json:"reply,omitempty"`
	Error          string         `json:"error,omitempty"`
	Messages       []chat.Message `json:"messages,omitempty"`
}

type secretaryChatHistoryLogger struct {
	path string
	mu   sync.Mutex
}

func newSecretaryChatHistoryLogger(path string) *secretaryChatHistoryLogger {
	path = filepath.Clean(path)
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return &secretaryChatHistoryLogger{path: path}
}

func (l *secretaryChatHistoryLogger) Append(entry secretaryChatHistoryLogEntry) error {
	if l == nil || strings.TrimSpace(l.path) == "" {
		return nil
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := bufio.NewWriterSize(f, 64*1024)
	if _, err := w.Write(append(line, '\n')); err != nil {
		return err
	}
	return w.Flush()
}

func listAllChatMessages(ctx context.Context, store *chat.Store) ([]chat.Message, error) {
	if store == nil {
		return nil, nil
	}
	var out []chat.Message
	var after int64
	for {
		batch, err := store.List(ctx, after, 500)
		if err != nil {
			return out, err
		}
		if len(batch) == 0 {
			return out, nil
		}
		out = append(out, batch...)
		after = batch[len(batch)-1].ID
	}
}
