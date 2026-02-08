package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Backend interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
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
