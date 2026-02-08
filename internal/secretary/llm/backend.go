package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"controlccx/internal/agentsdk"
)

type Backend interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

// ChatBackend is an optional extension interface for structured chat requests.
// Backends that support provider-specific features (e.g. prompt caching) SHOULD implement this.
type ChatBackend interface {
	Backend
	CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error)
}

// ChatStreamBackend is an optional extension interface for token/segment streaming.
// Backends SHOULD call callback for each visible assistant delta in order.
type ChatStreamBackend interface {
	ChatBackend
	CompleteChatStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error
}

// AutoBackend selects the first backend that successfully completes once, then sticks to it.
// It is best-effort: failures before selection will fall through to later backends.
type AutoBackend struct {
	Backends []Backend

	mu     sync.Mutex
	chosen Backend
}

func (b *AutoBackend) Name() string { return "auto" }

func (b *AutoBackend) Complete(ctx context.Context, prompt string) (string, error) {
	b.mu.Lock()
	chosen := b.chosen
	b.mu.Unlock()
	if chosen != nil {
		return chosen.Complete(ctx, prompt)
	}

	var errs []string
	for _, candidate := range b.Backends {
		if candidate == nil {
			continue
		}
		out, err := candidate.Complete(ctx, prompt)
		if err == nil && strings.TrimSpace(out) != "" {
			b.mu.Lock()
			if b.chosen == nil {
				b.chosen = candidate
			}
			b.mu.Unlock()
			return out, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", candidate.Name(), err))
		}
	}
	if len(errs) == 0 {
		return "", errors.New("no available LLM backends")
	}
	return "", fmt.Errorf("all LLM backends failed: %s", strings.Join(errs, "; "))
}

func (b *AutoBackend) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	b.mu.Lock()
	chosen := b.chosen
	b.mu.Unlock()
	if chosen != nil {
		if cb, ok := chosen.(ChatBackend); ok {
			return cb.CompleteChat(ctx, messages, opts)
		}
		return chosen.Complete(ctx, flattenMessages(messages))
	}

	var errs []string
	for _, candidate := range b.Backends {
		if candidate == nil {
			continue
		}
		var (
			out string
			err error
		)
		if cb, ok := candidate.(ChatBackend); ok {
			out, err = cb.CompleteChat(ctx, messages, opts)
		} else {
			out, err = candidate.Complete(ctx, flattenMessages(messages))
		}
		if err == nil && strings.TrimSpace(out) != "" {
			b.mu.Lock()
			if b.chosen == nil {
				b.chosen = candidate
			}
			b.mu.Unlock()
			return out, nil
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", candidate.Name(), err))
		}
	}
	if len(errs) == 0 {
		return "", errors.New("no available LLM backends")
	}
	return "", fmt.Errorf("all LLM backends failed: %s", strings.Join(errs, "; "))
}
