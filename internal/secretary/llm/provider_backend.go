package llm

import (
	"context"
	"errors"
	"strings"
	"sync"

	"controlccx/internal/agentsdk"
	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/providers"
)

// ProviderBackend routes Secretary LLM calls based on the active provider profile:
// targets.secretary.backend = simple-http | openai-chat
//
// It preserves backward compatibility by defaulting to simple-http when unset/unknown.
type ProviderBackend struct {
	providers *providers.Store

	simple Backend
	openai Backend

	receiptMu   sync.RWMutex
	lastReceipt map[string]any
}

var errNoProviderLLMBackends = errors.New("no available LLM backends")

func NewProviderBackend(cfg config.Config, authStore *auth.Store) Backend {
	return NewProviderBackendWithProviders(cfg, authStore, nil)
}

func NewProviderBackendWithProviders(cfg config.Config, authStore *auth.Store, providersStore *providers.Store) Backend {
	return &ProviderBackend{
		providers: providersStore,
		simple:    NewSimpleHTTPBackendWithProviders(cfg, authStore, providersStore),
		openai:    NewOpenAIChatBackendWithProviders(cfg, authStore, providersStore),
	}
}

func (b *ProviderBackend) Name() string {
	return b.currentBackendName()
}

func (b *ProviderBackend) LastReceipt() map[string]any {
	if b == nil {
		return nil
	}
	b.receiptMu.RLock()
	defer b.receiptMu.RUnlock()
	return cloneAnyMap(b.lastReceipt)
}

func (b *ProviderBackend) setLastReceipt(receipt map[string]any) {
	if b == nil {
		return
	}
	b.receiptMu.Lock()
	defer b.receiptMu.Unlock()
	b.lastReceipt = cloneAnyMap(receipt)
}

func (b *ProviderBackend) clearLastReceipt() {
	b.setLastReceipt(nil)
}

func (b *ProviderBackend) currentBackendName() string {
	if b == nil || b.providers == nil {
		return "simple-http"
	}
	active := b.providers.Active()
	id := strings.TrimSpace(active.Secretary)
	if id == "" {
		return "simple-http"
	}
	p, ok := b.providers.Get(id)
	if !ok {
		return "simple-http"
	}
	backend := strings.ToLower(strings.TrimSpace(p.Targets.Secretary.Backend))
	switch backend {
	case "openai-chat":
		return "openai-chat"
	case "", "simple-http":
		return "simple-http"
	default:
		return "simple-http"
	}
}

func (b *ProviderBackend) chosen() Backend {
	if b == nil {
		return nil
	}
	switch b.currentBackendName() {
	case "openai-chat":
		return b.openai
	default:
		return b.simple
	}
}

func (b *ProviderBackend) captureReceiptFrom(backend Backend) {
	if b == nil {
		return
	}
	type receiptProvider interface {
		LastReceipt() map[string]any
	}
	p, ok := backend.(receiptProvider)
	if !ok || p == nil {
		return
	}
	receipt := p.LastReceipt()
	if len(receipt) == 0 {
		return
	}
	b.setLastReceipt(receipt)
}

func (b *ProviderBackend) Complete(ctx context.Context, prompt string) (string, error) {
	b.clearLastReceipt()
	backend := b.chosen()
	if backend == nil {
		return "", errNoProviderLLMBackends
	}
	out, err := backend.Complete(ctx, prompt)
	b.captureReceiptFrom(backend)
	return out, err
}

func (b *ProviderBackend) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	b.clearLastReceipt()
	backend := b.chosen()
	if backend == nil {
		return "", errNoProviderLLMBackends
	}
	if cb, ok := backend.(ChatBackend); ok {
		out, err := cb.CompleteChat(ctx, messages, opts)
		b.captureReceiptFrom(backend)
		return out, err
	}
	out, err := backend.Complete(ctx, flattenMessages(messages))
	b.captureReceiptFrom(backend)
	return out, err
}

func (b *ProviderBackend) CompleteChatStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	b.clearLastReceipt()
	backend := b.chosen()
	if backend == nil {
		return callback("秘书不可用：未配置可用的 LLM backend。")
	}
	if csb, ok := backend.(ChatStreamBackend); ok {
		err := csb.CompleteChatStream(ctx, messages, opts, callback)
		b.captureReceiptFrom(backend)
		return err
	}
	if cb, ok := backend.(ChatBackend); ok {
		out, err := cb.CompleteChat(ctx, messages, opts)
		b.captureReceiptFrom(backend)
		if err != nil {
			return err
		}
		return callback(out)
	}
	out, err := backend.Complete(ctx, flattenMessages(messages))
	b.captureReceiptFrom(backend)
	if err != nil {
		return err
	}
	return callback(out)
}
